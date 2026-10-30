// Package audit appends structured action records to a JSONL file.
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is a single audited action.
type Entry struct {
	TS       string `json:"ts"`
	User     string `json:"user"`
	IP       string `json:"ip"`
	Action   string `json:"action"`
	Target   string `json:"target,omitempty"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// Logger writes audit entries to disk.
type Logger struct {
	mu   sync.Mutex
	path string
}

// New creates a logger bound to path.
func New(path string) *Logger {
	return &Logger{path: path}
}

// Write appends one entry. Errors are returned but callers may ignore them.
func (l *Logger) Write(e Entry) error {
	if e.TS == "" {
		e.TS = time.Now().Format(time.RFC3339)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

// Tail returns at most limit recent entries, newest last.
func (l *Logger) Tail(limit int) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := make([]string, 0, 64)
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	out := make([]Entry, 0, len(lines))
	for _, ln := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(ln), &e); err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}
