package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const FormatVersion = 1

type State struct {
	FormatVersion int        `json:"formatVersion"`
	DeviceID      string     `json:"deviceId"`
	Counter       uint64     `json:"counter"`
	Groups        []Group    `json:"groups"`
	Bookmarks     []Bookmark `json:"bookmarks"`
}

type Group struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
	Revision string `json:"revision"`
	Deleted  bool   `json:"deleted,omitempty"`
}

type Bookmark struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	GroupID   string   `json:"groupId,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
	Revision  string   `json:"revision"`
	Deleted   bool     `json:"deleted,omitempty"`
}

type Revision struct {
	Counter  uint64
	DeviceID string
}

func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

func ParseRevision(value string) Revision {
	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 {
		return Revision{}
	}
	counter, _ := strconv.ParseUint(parts[0], 10, 64)
	return Revision{Counter: counter, DeviceID: parts[1]}
}

func (r Revision) String() string {
	return fmt.Sprintf("%d@%s", r.Counter, r.DeviceID)
}

func CompareRevision(a, b string) int {
	ra, rb := ParseRevision(a), ParseRevision(b)
	if ra.Counter < rb.Counter {
		return -1
	}
	if ra.Counter > rb.Counter {
		return 1
	}
	return strings.Compare(ra.DeviceID, rb.DeviceID)
}

func Merge(states ...State) State {
	groups := map[string]Group{}
	bookmarks := map[string]Bookmark{}
	result := State{FormatVersion: FormatVersion}
	for _, state := range states {
		if state.Counter > result.Counter {
			result.Counter = state.Counter
		}
		for _, group := range state.Groups {
			current, ok := groups[group.ID]
			if !ok || CompareRevision(group.Revision, current.Revision) > 0 {
				groups[group.ID] = group
			}
		}
		for _, bookmark := range state.Bookmarks {
			current, ok := bookmarks[bookmark.ID]
			if !ok || CompareRevision(bookmark.Revision, current.Revision) > 0 {
				bookmarks[bookmark.ID] = bookmark
			}
		}
	}
	for _, group := range groups {
		result.Groups = append(result.Groups, group)
	}
	for _, bookmark := range bookmarks {
		result.Bookmarks = append(result.Bookmarks, bookmark)
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		if result.Groups[i].ParentID == result.Groups[j].ParentID {
			if result.Groups[i].Order == result.Groups[j].Order {
				return result.Groups[i].Name < result.Groups[j].Name
			}
			return result.Groups[i].Order < result.Groups[j].Order
		}
		return result.Groups[i].ParentID < result.Groups[j].ParentID
	})
	sort.Slice(result.Bookmarks, func(i, j int) bool {
		return result.Bookmarks[i].CreatedAt > result.Bookmarks[j].CreatedAt
	})
	return result
}

func NormalizeTags(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' }) {
			tag = strings.TrimSpace(tag)
			key := strings.ToLower(tag)
			if tag != "" && !seen[key] {
				seen[key] = true
				result = append(result, tag)
			}
		}
	}
	return result
}

func GroupPath(state State, id string) string {
	byID := map[string]Group{}
	for _, group := range state.Groups {
		if !group.Deleted {
			byID[group.ID] = group
		}
	}
	var names []string
	visited := map[string]bool{}
	for id != "" && !visited[id] {
		visited[id] = true
		group, ok := byID[id]
		if !ok {
			break
		}
		names = append([]string{group.Name}, names...)
		id = group.ParentID
	}
	return strings.Join(names, "/")
}
