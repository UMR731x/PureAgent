package studio

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Store struct {
	directory string
	mu        sync.Mutex
}

func newID() string { return rand.Text() }

func newStore(directory string) (*Store, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(absolute, "sessions"), 0700); err != nil {
		return nil, err
	}
	return &Store{directory: absolute}, nil
}

// Rename only a fully written file, so a stopped process cannot truncate JSON.
func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".saving-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(encoded, '\n'))
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func readJSON(path string, value any) error {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, value)
}

func (s *Store) config() (Config, error) {
	var config Config
	err := readJSON(filepath.Join(s.directory, "config.json"), &config)
	if errors.Is(err, os.ErrNotExist) {
		config.Models = []Model{{
			ID: "seedream", Name: "Seedream 5.5", ModelName: "doubao-seedream-5.5",
			URL: "https://ark.cn-beijing.volces.com/api/plan/v3/images/generations",
		}}
		return config, nil
	}
	return config, err
}

func (s *Store) sessionPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || id == "." || id == ".." || filepath.IsAbs(id) {
		return "", os.ErrNotExist
	}
	return filepath.Join(s.directory, "sessions", id), nil
}

func (s *Store) session(id string) (*Session, error) {
	directory, err := s.sessionPath(id)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := readJSON(filepath.Join(directory, "session.json"), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) saveSession(session *Session) error {
	directory, err := s.sessionPath(session.ID)
	if err != nil {
		return err
	}
	session.UpdatedAt = time.Now().UTC()
	return writeJSON(filepath.Join(directory, "session.json"), session)
}

func (s *Store) createSession() (*Session, error) {
	now := time.Now().UTC()
	session := &Session{
		ID: newID(), Title: "新任务", CreatedAt: now, UpdatedAt: now,
		Assets: []Asset{}, Turns: []Turn{},
	}
	directory, _ := s.sessionPath(session.ID)
	if err := os.MkdirAll(filepath.Join(directory, "sources"), 0700); err != nil {
		return nil, err
	}
	return session, s.saveSession(session)
}

func (s *Store) sessions() ([]SessionSummary, error) {
	entries, err := os.ReadDir(filepath.Join(s.directory, "sessions"))
	if err != nil {
		return nil, err
	}
	summaries := []SessionSummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := s.session(entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue // An interrupted first save can leave an empty session directory.
		}
		if err != nil {
			return nil, fmt.Errorf("读取任务 %s：%w", entry.Name(), err)
		}
		summary := SessionSummary{ID: session.ID, Title: session.Title, UpdatedAt: session.UpdatedAt}
		for _, turn := range session.Turns {
			summary.Running = summary.Running || turn.Status == "running"
			if len(turn.Outputs) > 0 {
				summary.Thumbnail = turn.Outputs[0].AssetID
			}
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt) })
	return summaries, nil
}

func (s *Store) recoverInterrupted() error {
	summaries, err := s.sessions()
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		if !summary.Running {
			continue
		}
		session, err := s.session(summary.ID)
		if err != nil {
			return err
		}
		for i := range session.Turns {
			turn := &session.Turns[i]
			if turn.Status == "running" {
				now := time.Now().UTC()
				turn.Status, turn.FinishedAt = "interrupted", &now
				turn.Error = &APIError{Message: "程序在生成期间退出。已保存的图片仍可使用；上游请求可能已计费，请确认后再生成。"}
			}
		}
		if err := s.saveSession(session); err != nil {
			return err
		}
	}
	return nil
}
