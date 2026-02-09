package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session represents a conversation session.
type Session struct {
	Key       string                   `json:"key"`
	Messages  []map[string]interface{} `json:"messages"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	Metadata  map[string]interface{}   `json:"metadata"`
}

// NewSession creates a new session.
func NewSession(key string) *Session {
	return &Session{
		Key:       key,
		Messages:  make([]map[string]interface{}, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// AddMessage adds a message to the session.
func (s *Session) AddMessage(role string, content string, extra map[string]interface{}) {
	msg := map[string]interface{}{
		"role":      role,
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	for k, v := range extra {
		msg[k] = v
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory returns message history for LLM context.
func (s *Session) GetHistory(maxMessages int) []map[string]interface{} {
	msgs := s.Messages
	if len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}

	history := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		history = append(history, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}
	return history
}

// Manager manages conversation sessions.
type Manager struct {
	Workspace   string
	SessionsDir string
	cache       map[string]*Session
	mu          sync.RWMutex
}

// NewManager creates a new session manager.
func NewManager(workspace string) *Manager {
	sessionsDir := filepath.Join(workspace, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	return &Manager{
		Workspace:   workspace,
		SessionsDir: sessionsDir,
		cache:       make(map[string]*Session),
	}
}

func (m *Manager) getSessionPath(key string) string {
	safeKey := strings.ReplaceAll(key, ":", "_")
	// Sanitize filename further if needed
	return filepath.Join(m.SessionsDir, safeKey+".jsonl")
}

// GetOrCreate gets an existing session or creates a new one.
func (m *Manager) GetOrCreate(key string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.cache[key]; ok {
		return session
	}

	session := m.load(key)
	if session == nil {
		session = NewSession(key)
	}

	m.cache[key] = session
	return session
}

func (m *Manager) load(key string) *Session {
	path := m.getSessionPath(key)
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	session := NewSession(key)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		if typeVal, ok := data["_type"]; ok && typeVal == "metadata" {
			if meta, ok := data["metadata"].(map[string]interface{}); ok {
				session.Metadata = meta
			}
			if created, ok := data["created_at"].(string); ok {
				t, _ := time.Parse(time.RFC3339, created)
				session.CreatedAt = t
			}
		} else {
			session.Messages = append(session.Messages, data)
		}
	}

	return session
}

// Save saves a session to disk.
func (m *Manager) Save(session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache[session.Key] = session
	path := m.getSessionPath(session.Key)

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write metadata
	metaLine := map[string]interface{}{
		"_type":      "metadata",
		"created_at": session.CreatedAt.Format(time.RFC3339),
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
		"metadata":   session.Metadata,
	}

	metaJSON, _ := json.Marshal(metaLine)
	file.WriteString(string(metaJSON) + "\n")

	// Write messages
	for _, msg := range session.Messages {
		msgJSON, _ := json.Marshal(msg)
		file.WriteString(string(msgJSON) + "\n")
	}

	return nil
}

// Clear clears a session.
func (m *Manager) Clear(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.cache, key)
	path := m.getSessionPath(key)
	return os.Remove(path)
}
