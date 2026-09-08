package server

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bookmarkhubgo/internal/model"
	"bookmarkhubgo/internal/storage"
)

//go:embed web/*
var webFiles embed.FS

var Version = "dev"

type Server struct {
	store *storage.Store
	mux   *http.ServeMux
}

func New(store *storage.Store) *Server {
	server := &Server{store: store, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.security(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/status", s.status)
	s.mux.HandleFunc("GET /api/v1/state", s.state)
	s.mux.HandleFunc("POST /api/v1/bookmarks", s.upsertBookmark)
	s.mux.HandleFunc("POST /api/v1/bookmarks/star", s.starBookmark)
	s.mux.HandleFunc("POST /api/v1/bookmarks/delete", s.deleteBookmark)
	s.mux.HandleFunc("POST /api/v1/groups", s.upsertGroup)
	s.mux.HandleFunc("POST /api/v1/groups/delete", s.deleteGroup)
	s.mux.HandleFunc("POST /api/v1/import", s.importData)
	s.mux.HandleFunc("GET /api/v1/export", s.exportData)
	s.mux.HandleFunc("POST /api/v1/settings/sync-dir", s.setSyncDir)
	root, _ := fs.Sub(webFiles, "web")
	s.mux.Handle("/", http.FileServer(http.FS(root)))
}

func (s *Server) starBookmark(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID      string `json:"id"`
		Starred bool   `json:"starred"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.SetBookmarkStar(input.ID, input.Starred); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		isExtension := strings.HasPrefix(origin, "chrome-extension://") || strings.HasPrefix(origin, "extension://")
		isLocalUI := origin == "" || strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "http://localhost:")
		if origin != "" && !isExtension && !isLocalUI {
			http.Error(w, "origin denied", http.StatusForbidden)
			return
		}
		if isExtension {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-BookmarkHub-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && isExtension && r.Header.Get("X-BookmarkHub-Token") != s.store.Settings().Token {
			http.Error(w, "pairing token is invalid", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	settings := s.store.Settings()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "version": Version, "deviceId": settings.DeviceID,
		"deviceFile": s.store.DeviceFilename(), "syncDir": settings.SyncDir,
		"pairingToken": settings.Token,
	})
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	state, err := s.store.Load()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicState(state))
}

func (s *Server) upsertBookmark(w http.ResponseWriter, r *http.Request) {
	var input storage.BookmarkInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	bookmark, err := s.store.UpsertBookmark(input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookmark)
}

func (s *Server) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.DeleteBookmark(input.ID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) upsertGroup(w http.ResponseWriter, r *http.Request) {
	var input storage.GroupInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	group, err := s.store.UpsertGroup(input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.DeleteGroup(input.ID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) importData(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Format  string `json:"format"`
		Mode    string `json:"mode"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	if input.Mode != "merge" && input.Mode != "replace" {
		writeError(w, errors.New("mode must be merge or replace"))
		return
	}
	state, err := s.store.Import(input.Format, input.Mode, []byte(input.Content))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicState(state))
}

func (s *Server) exportData(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.Load()
	if err != nil {
		writeError(w, err)
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	var content []byte
	var filename string
	if format == "xbel" {
		content, err = storage.EncodeXBEL(state)
		filename = "bookmarkhub-export.xbel"
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	} else {
		content, err = storage.EncodeHTML(state)
		filename = "bookmarks.html"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	_, _ = w.Write(content)
}

func (s *Server) setSyncDir(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.SetSyncDir(input.Path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func publicState(state model.State) model.State {
	state.Groups = filterGroups(state.Groups)
	state.Bookmarks = filterBookmarks(state.Bookmarks)
	return state
}

func filterGroups(values []model.Group) []model.Group {
	result := make([]model.Group, 0, len(values))
	for _, value := range values {
		if !value.Deleted {
			result = append(result, value)
		}
	}
	return result
}

func filterBookmarks(values []model.Bookmark) []model.Bookmark {
	result := make([]model.Bookmark, 0, len(values))
	for _, value := range values {
		if !value.Deleted {
			result = append(result, value)
		}
	}
	return result
}

func readJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, fs.ErrNotExist) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
