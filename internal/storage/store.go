package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"bookmarkhubgo/internal/model"
)

type Settings struct {
	DeviceID string `json:"deviceId"`
	Token    string `json:"token"`
	SyncDir  string `json:"syncDir"`
	Port     int    `json:"port"`
}

type Store struct {
	mu       sync.Mutex
	home     string
	settings Settings
}

type BookmarkInput struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	GroupID   string   `json:"groupId"`
	GroupPath string   `json:"groupPath"`
	Tags      []string `json:"tags"`
	Notes     string   `json:"notes"`
	Starred   *bool    `json:"starred,omitempty"`
	Color     *string  `json:"color,omitempty"`
}

type GroupInput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

func Open(home, syncOverride string, port int) (*Store, error) {
	if home == "" {
		home = "."
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(absHome, "config"), 0o755); err != nil {
		return nil, err
	}
	settingsPath := filepath.Join(absHome, "config", "settings.json")
	settings := Settings{Port: port}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, fmt.Errorf("read settings: %w", err)
		}
	}
	if settings.DeviceID == "" {
		settings.DeviceID = randomHex(8)
	}
	if settings.Token == "" {
		settings.Token = randomHex(24)
	}
	if syncOverride != "" {
		settings.SyncDir = syncOverride
	}
	if settings.SyncDir == "" {
		settings.SyncDir = filepath.Join(absHome, "data", "sync")
	}
	if settings.Port == 0 {
		settings.Port = 17836
	}
	settings.SyncDir, err = filepath.Abs(settings.SyncDir)
	if err != nil {
		return nil, err
	}
	store := &Store{home: absHome, settings: settings}
	if err := store.saveSettings(); err != nil {
		return nil, err
	}
	if err := store.ensureDirectories(); err != nil {
		return nil, err
	}
	if _, err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func (s *Store) DeviceFilename() string {
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}
	return fmt.Sprintf("bookmarkhub-%s-%s.html", osName, s.settings.DeviceID)
}

func (s *Store) SetSyncDir(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("sync directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.settings.SyncDir = abs
	if err := s.ensureDirectoriesUnlocked(); err != nil {
		return err
	}
	return s.saveSettingsUnlocked()
}

func (s *Store) Load() (model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *Store) loadUnlocked() (model.State, error) {
	deviceDir := filepath.Join(s.settings.SyncDir, "devices")
	entries, err := os.ReadDir(deviceDir)
	if err != nil {
		return model.State{}, err
	}
	var states []model.State
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".html") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(deviceDir, entry.Name()))
		if err != nil {
			return model.State{}, err
		}
		state, err := DecodeHTML(content)
		if err != nil {
			return model.State{}, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		states = append(states, state)
	}
	merged := model.Merge(states...)
	merged.DeviceID = s.settings.DeviceID
	if len(states) == 0 {
		if err := s.writeOwnUnlocked(merged); err != nil {
			return model.State{}, err
		}
	}
	return merged, nil
}

func (s *Store) UpsertBookmark(input BookmarkInput) (model.Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadUnlocked()
	if err != nil {
		return model.Bookmark{}, err
	}
	input.URL = strings.TrimSpace(input.URL)
	if parsed, err := url.ParseRequestURI(input.URL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return model.Bookmark{}, errors.New("URL must use http or https")
	}
	color := ""
	if input.Color != nil {
		color = strings.ToLower(strings.TrimSpace(*input.Color))
		if !validBookmarkColor(color) {
			return model.Bookmark{}, errors.New("color must be empty, red, orange, yellow, green, cyan, blue, purple, or pink")
		}
	}
	groupID := input.GroupID
	if input.GroupPath != "" {
		groupID = s.ensureGroupPath(&state, input.GroupPath)
	}
	index := -1
	for i := range state.Bookmarks {
		if (input.ID != "" && state.Bookmarks[i].ID == input.ID) || (input.ID == "" && !state.Bookmarks[i].Deleted && state.Bookmarks[i].URL == input.URL) {
			index = i
			break
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state.Counter++
	bookmark := model.Bookmark{ID: input.ID, URL: input.URL, Title: strings.TrimSpace(input.Title), GroupID: groupID, Tags: model.NormalizeTags(input.Tags), Notes: strings.TrimSpace(input.Notes), Color: color, UpdatedAt: now, Revision: model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()}
	if input.Starred != nil {
		bookmark.Starred = *input.Starred
	}
	if bookmark.ID == "" {
		bookmark.ID = model.NewID("b")
	}
	if bookmark.Title == "" {
		bookmark.Title = bookmark.URL
	}
	if index >= 0 {
		bookmark.ID = state.Bookmarks[index].ID
		bookmark.CreatedAt = state.Bookmarks[index].CreatedAt
		if input.Starred == nil {
			bookmark.Starred = state.Bookmarks[index].Starred
		}
		if input.Color == nil {
			bookmark.Color = state.Bookmarks[index].Color
		}
		state.Bookmarks[index] = bookmark
	} else {
		bookmark.CreatedAt = now
		state.Bookmarks = append(state.Bookmarks, bookmark)
	}
	if err := s.writeOwnUnlocked(state); err != nil {
		return model.Bookmark{}, err
	}
	return bookmark, nil
}

func validBookmarkColor(color string) bool {
	switch color {
	case "", "red", "orange", "yellow", "green", "cyan", "blue", "purple", "pink":
		return true
	default:
		return false
	}
}

func (s *Store) SetBookmarkStar(id string, starred bool) error {
	return s.mutate(func(state *model.State) error {
		for i := range state.Bookmarks {
			if state.Bookmarks[i].ID == id && !state.Bookmarks[i].Deleted {
				state.Counter++
				state.Bookmarks[i].Starred = starred
				state.Bookmarks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				state.Bookmarks[i].Revision = model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()
				return nil
			}
		}
		return os.ErrNotExist
	})
}

func (s *Store) DeleteBookmark(id string) error {
	return s.mutate(func(state *model.State) error {
		for i := range state.Bookmarks {
			if state.Bookmarks[i].ID == id {
				state.Counter++
				state.Bookmarks[i].Deleted = true
				state.Bookmarks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				state.Bookmarks[i].Revision = model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()
				return nil
			}
		}
		return os.ErrNotExist
	})
}

func (s *Store) UpsertGroup(input GroupInput) (model.Group, error) {
	var result model.Group
	err := s.mutate(func(state *model.State) error {
		if strings.TrimSpace(input.Name) == "" {
			return errors.New("group name is required")
		}
		name := strings.TrimSpace(input.Name)
		for i := range state.Groups {
			if state.Groups[i].ID != input.ID {
				continue
			}
			order := state.Groups[i].Order
			if state.Groups[i].ParentID != input.ParentID {
				order = nextGroupOrder(state, input.ParentID, input.ID)
			}
			state.Counter++
			result = model.Group{ID: input.ID, Name: name, ParentID: input.ParentID, Order: order, Revision: model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()}
			state.Groups[i] = result
			return nil
		}
		state.Counter++
		result = model.Group{ID: model.NewID("g"), Name: name, ParentID: input.ParentID, Order: nextGroupOrder(state, input.ParentID, ""), Revision: model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()}
		state.Groups = append(state.Groups, result)
		return nil
	})
	return result, err
}

func (s *Store) ReorderGroups(parentID string, ids []string) error {
	return s.mutate(func(state *model.State) error {
		indexes := map[string]int{}
		for i := range state.Groups {
			group := state.Groups[i]
			if !group.Deleted && group.ParentID == parentID {
				indexes[group.ID] = i
			}
		}
		if len(ids) != len(indexes) {
			return errors.New("group order must contain every sibling exactly once")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			index, ok := indexes[id]
			if !ok || seen[id] {
				return errors.New("group order contains an unknown or duplicate group")
			}
			seen[id] = true
			if state.Groups[index].Order == len(seen)-1 {
				continue
			}
			state.Counter++
			state.Groups[index].Order = len(seen) - 1
			state.Groups[index].Revision = model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()
		}
		return nil
	})
}

func nextGroupOrder(state *model.State, parentID, excludeID string) int {
	maxOrder := -1
	for _, group := range state.Groups {
		if !group.Deleted && group.ID != excludeID && group.ParentID == parentID && group.Order > maxOrder {
			maxOrder = group.Order
		}
	}
	return maxOrder + 1
}

func (s *Store) DeleteGroup(id string) error {
	return s.mutate(func(state *model.State) error {
		var parent string
		found := false
		state.Counter++
		revision := model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()
		for i := range state.Groups {
			if state.Groups[i].ID == id {
				parent = state.Groups[i].ParentID
				state.Groups[i].Deleted = true
				state.Groups[i].Revision = revision
				found = true
			}
		}
		if !found {
			return os.ErrNotExist
		}
		for i := range state.Groups {
			if state.Groups[i].ParentID == id && !state.Groups[i].Deleted {
				state.Groups[i].ParentID = parent
				state.Groups[i].Revision = revision
			}
		}
		for i := range state.Bookmarks {
			if state.Bookmarks[i].GroupID == id && !state.Bookmarks[i].Deleted {
				state.Bookmarks[i].GroupID = parent
				state.Bookmarks[i].Revision = revision
			}
		}
		return nil
	})
}

func (s *Store) Import(format, mode string, content []byte) (model.State, error) {
	var incoming model.State
	var err error
	switch strings.ToLower(format) {
	case "html", "htm":
		incoming, err = DecodeHTML(content)
	case "xbel", "xml":
		incoming, err = DecodeXBEL(content)
	default:
		return model.State{}, errors.New("format must be html or xbel")
	}
	if err != nil {
		return model.State{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.loadUnlocked()
	if err != nil {
		return model.State{}, err
	}
	if mode == "replace" {
		for i := range current.Groups {
			current.Counter++
			current.Groups[i].Deleted = true
			current.Groups[i].Revision = model.Revision{Counter: current.Counter, DeviceID: s.settings.DeviceID}.String()
		}
		for i := range current.Bookmarks {
			current.Counter++
			current.Bookmarks[i].Deleted = true
			current.Bookmarks[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			current.Bookmarks[i].Revision = model.Revision{Counter: current.Counter, DeviceID: s.settings.DeviceID}.String()
		}
	}
	for i := range incoming.Groups {
		current.Counter++
		incoming.Groups[i].Revision = model.Revision{Counter: current.Counter, DeviceID: s.settings.DeviceID}.String()
	}
	for i := range incoming.Bookmarks {
		current.Counter++
		incoming.Bookmarks[i].Revision = model.Revision{Counter: current.Counter, DeviceID: s.settings.DeviceID}.String()
	}
	if mode == "replace" {
		current.Groups = append(current.Groups, incoming.Groups...)
		current.Bookmarks = append(current.Bookmarks, incoming.Bookmarks...)
	} else {
		current = mergeImported(current, incoming)
	}
	current.DeviceID = s.settings.DeviceID
	if err := s.writeOwnUnlocked(current); err != nil {
		return model.State{}, err
	}
	return current, nil
}

func (s *Store) mutate(fn func(*model.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	if err := fn(&state); err != nil {
		return err
	}
	return s.writeOwnUnlocked(state)
}

func (s *Store) ensureGroupPath(state *model.State, path string) string {
	parent := ""
	for _, name := range strings.Split(path, "/") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		found := ""
		for _, group := range state.Groups {
			if !group.Deleted && group.ParentID == parent && group.Name == name {
				found = group.ID
				break
			}
		}
		if found == "" {
			state.Counter++
			found = model.NewID("g")
			state.Groups = append(state.Groups, model.Group{ID: found, Name: name, ParentID: parent, Order: len(state.Groups), Revision: model.Revision{Counter: state.Counter, DeviceID: s.settings.DeviceID}.String()})
		}
		parent = found
	}
	return parent
}

func (s *Store) writeOwnUnlocked(state model.State) error {
	state.FormatVersion = model.FormatVersion
	state.DeviceID = s.settings.DeviceID
	content, err := EncodeHTML(state)
	if err != nil {
		return err
	}
	target := filepath.Join(s.settings.SyncDir, "devices", s.DeviceFilename())
	if old, err := os.ReadFile(target); err == nil && len(old) > 0 {
		backupDir := filepath.Join(s.settings.SyncDir, "backups")
		_ = os.MkdirAll(backupDir, 0o755)
		backup := filepath.Join(backupDir, time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+s.DeviceFilename())
		_ = os.WriteFile(backup, old, 0o644)
		_ = trimBackups(backupDir, s.DeviceFilename(), 20)
	}
	return atomicWrite(target, content, 0o644)
}

func (s *Store) ensureDirectories() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureDirectoriesUnlocked()
}

func (s *Store) ensureDirectoriesUnlocked() error {
	for _, name := range []string{"devices", "exports", "backups"} {
		if err := os.MkdirAll(filepath.Join(s.settings.SyncDir, name), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) saveSettings() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSettingsUnlocked()
}

func (s *Store) saveSettingsUnlocked() error {
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.home, "config", "settings.json"), append(data, '\n'), 0o600)
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	temp := path + ".tmp-" + randomHex(4)
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(content); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temp)
		return err
	}
	return replaceFile(temp, path)
}

func replaceFile(temp, target string) error {
	backup := target + ".replace-backup"
	_ = os.Remove(backup)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			_ = os.Remove(temp)
			return err
		}
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Rename(backup, target)
		_ = os.Remove(temp)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func randomHex(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(data)
}

func mergeImported(current, incoming model.State) model.State {
	groupIDs := map[string]bool{}
	for _, group := range current.Groups {
		groupIDs[group.ID] = true
	}
	for _, group := range incoming.Groups {
		if !groupIDs[group.ID] {
			current.Groups = append(current.Groups, group)
			groupIDs[group.ID] = true
		}
	}
	byURL := map[string]int{}
	for i, bookmark := range current.Bookmarks {
		if !bookmark.Deleted {
			byURL[bookmark.URL] = i
		}
	}
	for _, bookmark := range incoming.Bookmarks {
		if index, ok := byURL[bookmark.URL]; ok {
			bookmark.ID = current.Bookmarks[index].ID
			bookmark.CreatedAt = current.Bookmarks[index].CreatedAt
			current.Bookmarks[index] = bookmark
		} else {
			current.Bookmarks = append(current.Bookmarks, bookmark)
			byURL[bookmark.URL] = len(current.Bookmarks) - 1
		}
	}
	if incoming.Counter > current.Counter {
		current.Counter = incoming.Counter
	}
	return current
}

func trimBackups(dir, suffix string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return nil
}
