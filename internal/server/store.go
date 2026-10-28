package server

// Sessions survive a restart of movies-cast: before this, every update or
// reboot of the NAS sent the TV back to the pairing screen (seen on the TV
// 2026-10-28). The file holds Movies tokens, so it is written 0600 and only
// when STATE_FILE is set; a token stays revocable at Movies, and logout
// removes it from the file.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"movies-cast/internal/movies"
)

// seenSaveEvery bounds how stale a stored lastSeen may be; without it a
// session in daily use but never re-saved would look 30 days old after a
// restart and be dropped.
const seenSaveEvery = time.Hour

type storedSession struct {
	ID       string    `json:"id"`
	Token    string    `json:"token"`
	UserID   string    `json:"userId"`
	UserName string    `json:"userName"`
	DeviceID string    `json:"deviceId"`
	LastSeen time.Time `json:"lastSeen"`
}

// loadSessions reads the state file; a missing file is an empty store.
func (s *Server) loadSessions() error {
	if s.cfg.StateFile == "" {
		return nil
	}
	b, err := os.ReadFile(s.cfg.StateFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []storedSession
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("%s: %w", s.cfg.StateFile, err)
	}
	byToken := map[string]*movies.Session{} // a paired TV shares the phone's login
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range list {
		if st.ID == "" || st.Token == "" || s.now().Sub(st.LastSeen) > sessionMaxAge {
			continue
		}
		ms := byToken[st.Token]
		if ms == nil {
			ms = &movies.Session{Token: st.Token, UserID: st.UserID, UserName: st.UserName, DeviceID: st.DeviceID}
			byToken[st.Token] = ms
		}
		s.sessions[st.ID] = &session{ms: ms, lastSeen: st.LastSeen, savedSeen: st.LastSeen}
	}
	return nil
}

// saveSessionsLocked writes all sessions atomically. Caller holds s.mu.
func (s *Server) saveSessionsLocked() {
	if s.cfg.StateFile == "" {
		return
	}
	list := make([]storedSession, 0, len(s.sessions))
	for id, ss := range s.sessions {
		list = append(list, storedSession{ID: id, Token: ss.ms.Token, UserID: ss.ms.UserID, UserName: ss.ms.UserName, DeviceID: ss.ms.DeviceID, LastSeen: ss.lastSeen})
		ss.savedSeen = ss.lastSeen
	}
	b, err := json.Marshal(list)
	if err != nil {
		log.Printf("state: %v", err)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.cfg.StateFile), ".sessions-*")
	if err != nil {
		log.Printf("state: %v", err)
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		log.Printf("state: write: %v %v", werr, cerr)
		return
	}
	// CreateTemp makes the file 0600 already; Chmod states it.
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		log.Printf("state: chmod: %v", err)
	}
	if err := os.Rename(tmp.Name(), s.cfg.StateFile); err != nil {
		_ = os.Remove(tmp.Name())
		log.Printf("state: rename: %v", err)
	}
}

// Proven formats (TV|container|codec, see provenAfter) live next to the
// sessions in formats.json: without it the format hint came back for the LG
// after every restart. Nothing secret in it; written 0600 like its neighbour.
func (s *Server) formatsFile() string {
	if s.cfg.StateFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.cfg.StateFile), "formats.json")
}

func (s *Server) failedFile() string {
	if s.cfg.StateFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.cfg.StateFile), "formats-failed.json")
}

func (s *Server) loadProven() error {
	if err := loadKeys(s.formatsFile(), &s.mu, s.proven); err != nil {
		return err
	}
	return loadKeys(s.failedFile(), &s.mu, s.failed)
}

// loadKeys reads a JSON list of keys into set; a missing file is empty.
func loadKeys(f string, mu *sync.Mutex, set map[string]bool) error {
	if f == "" {
		return nil
	}
	b, err := os.ReadFile(f)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var keys []string
	if err := json.Unmarshal(b, &keys); err != nil {
		return fmt.Errorf("%s: %w", f, err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, k := range keys {
		set[k] = true
	}
	return nil
}

// saveProvenLocked writes the proven and the failed formats. Caller holds s.mu.
func (s *Server) saveProvenLocked() {
	saveKeys(s.formatsFile(), s.proven)
	saveKeys(s.failedFile(), s.failed)
}

// saveKeys writes set as a sorted JSON list, atomically.
func saveKeys(f string, set map[string]bool) {
	if f == "" {
		return
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, err := json.Marshal(keys)
	if err != nil {
		log.Printf("formats: %v", err)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(f), ".formats-*")
	if err != nil {
		log.Printf("formats: %v", err)
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		log.Printf("formats: write: %v", errors.Join(werr, cerr))
		return
	}
	if err := os.Rename(tmp.Name(), f); err != nil {
		_ = os.Remove(tmp.Name())
		log.Printf("formats: rename: %v", err)
	}
}
