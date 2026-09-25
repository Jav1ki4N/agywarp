package process

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store handles persistence of process profiles to and from disk.
type Store struct {
	Path string
}

// DefaultProfilesPath returns the default path ~/.config/agywarp/profiles.json.
func DefaultProfilesPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving user home/config dir: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "agywarp", "profiles.json"), nil
}

// NewStore creates a new Store instance pointing to path or default location.
func NewStore(customPath ...string) (*Store, error) {
	var targetPath string
	if len(customPath) > 0 && customPath[0] != "" {
		targetPath = customPath[0]
	} else {
		def, err := DefaultProfilesPath()
		if err != nil {
			return nil, err
		}
		targetPath = def
	}
	return &Store{Path: targetPath}, nil
}

// Load reads profiles from disk. If the file does not exist, it initializes
// and saves the default profiles, returning them.
func (s *Store) Load() ([]Profile, error) {
	if _, err := os.Stat(s.Path); os.IsNotExist(err) {
		defaults := DefaultProfiles()
		if err := s.Save(defaults); err != nil {
			return defaults, fmt.Errorf("initializing default profiles at %s: %w", s.Path, err)
		}
		return defaults, nil
	}

	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("reading profiles file %s: %w", s.Path, err)
	}

	var profiles []Profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parsing profiles JSON from %s: %w", s.Path, err)
	}

	return profiles, nil
}

// Save writes profiles atomically to disk formatted as indented JSON.
func (s *Store) Save(profiles []Profile) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profiles: %w", err)
	}
	data = append(data, '\n')

	// Write to temporary file in the same directory for atomic rename
	tmpFile, err := os.CreateTemp(dir, "profiles-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing to temp file %s: %w", tmpName, err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("syncing temp file %s: %w", tmpName, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, s.Path); err != nil {
		return fmt.Errorf("atomic rename %s -> %s: %w", tmpName, s.Path, err)
	}

	return nil
}
