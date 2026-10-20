package profile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// ErrNilProfile is returned when Resolve is called with a nil profile.
var ErrNilProfile = fmt.Errorf("profile: nil style profile")

// ErrNotFound is returned when a profile name is unknown.
func ErrNotFound(name string) error {
	return fmt.Errorf("profile: not found %q", name)
}

// Loader caches parsed style profiles by name.
type Loader struct {
	mu       sync.RWMutex
	profiles map[string]*StyleProfile
}

// NewLoader creates an empty profile loader.
func NewLoader() *Loader {
	return &Loader{profiles: make(map[string]*StyleProfile)}
}

// LoadDir loads all *.yaml / *.yml files from a filesystem directory.
func (l *Loader) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("profile load dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("profile read %s: %w", e.Name(), err)
		}
		if err := l.loadBytes(data); err != nil {
			return fmt.Errorf("profile parse %s: %w", e.Name(), err)
		}
	}
	return nil
}

// LoadFS loads all *.yaml / *.yml from an fs.FS (e.g. embed.FS).
func (l *Loader) LoadFS(fsys fs.FS, root string) error {
	return fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("profile read %s: %w", path, err)
		}
		if err := l.loadBytes(data); err != nil {
			return fmt.Errorf("profile parse %s: %w", path, err)
		}
		return nil
	})
}

func (l *Loader) loadBytes(data []byte) error {
	var p StyleProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Name == "" {
		return fmt.Errorf("missing name field")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.profiles[p.Name] = &p
	return nil
}

// Get returns a profile by name or ErrNotFound.
func (l *Loader) Get(name string) (*StyleProfile, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	p, ok := l.profiles[name]
	if !ok {
		return nil, ErrNotFound(name)
	}
	return p, nil
}

// Names returns loaded profile names.
func (l *Loader) Names() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	names := make([]string, 0, len(l.profiles))
	for n := range l.profiles {
		names = append(names, n)
	}
	return names
}

// MustLoadEmbedded is set by the embed package in profiles_embed.go when available.
// DefaultLoader returns a loader with built-in profiles from the profiles/ directory next to the module,
// or from embed if registered.
var defaultOnce sync.Once
var defaultLoader *Loader
var defaultErr error

// Default returns a process-wide loader with embedded/builtin profiles.
func Default() (*Loader, error) {
	defaultOnce.Do(func() {
		defaultLoader = NewLoader()
		if embedFS != nil {
			defaultErr = defaultLoader.LoadFS(embedFS, ".")
			return
		}
		defaultErr = fmt.Errorf("profile: no embedded profiles registered")
	})
	return defaultLoader, defaultErr
}

// embedFS is set by init in embed.go.
var embedFS fs.FS
