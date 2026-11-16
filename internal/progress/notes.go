package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Notes are the user's own notes per exercise, in notes.json. They are kept
// apart from progress: resetting an exercise's progress keeps its notes.
type Notes struct {
	path string
	mu   sync.Mutex
	data map[string]string
}

// OpenNotes loads (or starts) dir/notes.json. It always returns a usable
// store; on an error it starts empty.
func OpenNotes(dir string) (*Notes, error) {
	n := &Notes{path: filepath.Join(dir, "notes.json"), data: map[string]string{}}
	b, err := os.ReadFile(n.path)
	if err == nil {
		if err := json.Unmarshal(b, &n.data); err != nil {
			n.data = map[string]string{}
			return n, err // usable, but empty: the caller decides
		}
	} else if !os.IsNotExist(err) {
		return n, err
	}
	return n, nil
}

// Get returns the note for id.
func (n *Notes) Get(id string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.data[id]
}

// Set stores the note for id; an empty note is removed.
func (n *Notes) Set(id, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if strings.TrimSpace(text) == "" {
		delete(n.data, id)
	} else {
		n.data[id] = text
	}
	b, _ := json.MarshalIndent(n.data, "", "  ")
	tmp := n.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, n.path)
}

// Has reports whether id has a note.
func (n *Notes) Has(id string) bool { return n.Get(id) != "" }
