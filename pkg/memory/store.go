package memory

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MemoryStore manages persistent agent memory.
type MemoryStore struct {
	Workspace string
	MemoryDir string
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	os.MkdirAll(memoryDir, 0755)
	return &MemoryStore{
		Workspace: workspace,
		MemoryDir: memoryDir,
	}
}

// GetTodayFile returns the path to today's memory file.
func (m *MemoryStore) GetTodayFile() string {
	today := time.Now().Format("2006-01-02")
	return filepath.Join(m.MemoryDir, fmt.Sprintf("%s.md", today))
}

// ReadToday reads today's memory notes.
func (m *MemoryStore) ReadToday() (string, error) {
	path := m.GetTodayFile()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// AppendToday appends content to today's memory notes.
func (m *MemoryStore) AppendToday(content string) error {
	path := m.GetTodayFile()

	existing := ""
	if _, err := os.Stat(path); err == nil {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		existing = string(data)
		content = existing + "\n" + content
	} else {
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		content = header + content
	}

	return ioutil.WriteFile(path, []byte(content), 0644)
}

// ReadLongTerm reads long-term memory (MEMORY.md).
func (m *MemoryStore) ReadLongTerm() (string, error) {
	path := filepath.Join(m.MemoryDir, "MEMORY.md")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// WriteLongTerm writes to long-term memory (MEMORY.md).
func (m *MemoryStore) WriteLongTerm(content string) error {
	path := filepath.Join(m.MemoryDir, "MEMORY.md")
	return ioutil.WriteFile(path, []byte(content), 0644)
}

// GetRecentMemories returns memories from the last N days.
func (m *MemoryStore) GetRecentMemories(days int) (string, error) {
	var memories []string
	today := time.Now()

	for i := 0; i < days; i++ {
		date := today.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		path := filepath.Join(m.MemoryDir, fmt.Sprintf("%s.md", dateStr))

		if _, err := os.Stat(path); err == nil {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				return "", err
			}
			memories = append(memories, string(data))
		}
	}

	return joinStrings(memories, "\n\n---\n\n"), nil
}

// ListMemoryFiles lists all memory files sorted by date (newest first).
func (m *MemoryStore) ListMemoryFiles() ([]string, error) {
	files, err := ioutil.ReadDir(m.MemoryDir)
	if err != nil {
		return nil, err
	}

	var memoryFiles []string
	for _, f := range files {
		if !f.IsDir() && len(f.Name()) == 13 && f.Name()[10:] == ".md" {
			memoryFiles = append(memoryFiles, filepath.Join(m.MemoryDir, f.Name()))
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(memoryFiles)))
	return memoryFiles, nil
}

// GetMemoryContext returns the formatted memory context.
func (m *MemoryStore) GetMemoryContext() string {
	var parts []string

	longTerm, _ := m.ReadLongTerm()
	if longTerm != "" {
		parts = append(parts, "## Long-term Memory\n"+longTerm)
	}

	today, _ := m.ReadToday()
	if today != "" {
		parts = append(parts, "## Today's Notes\n"+today)
	}

	return joinStrings(parts, "\n\n")
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	res := strs[0]
	for i := 1; i < len(strs); i++ {
		res += sep + strs[i]
	}
	return res
}
