package deps

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// LockFile is the name of the file, beside yak.pkg, that pins the
// repositories a package's remote dependencies were vendored from.
const LockFile = "yak.lock"

// lockEntry pins one repository.
type lockEntry struct {
	Source string `yaml:"source"`
	Ref    string `yaml:"ref,omitempty"`
	Commit string `yaml:"commit"`
	Hash   string `yaml:"hash"`
}

type lockFile struct {
	// Packages is keyed by the repository's module.Source.Key.
	Packages map[string]lockEntry `yaml:"packages"`
}

func readLock(path string) (map[string]lockEntry, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]lockEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var lock lockFile
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	if err := dec.Decode(&lock); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if lock.Packages == nil {
		lock.Packages = map[string]lockEntry{}
	}
	return lock.Packages, nil
}

// writeLock writes entries to path, or removes path when there are none.
func writeLock(path string, entries map[string]lockEntry) error {
	if len(entries) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("# Written by yak sync. Do not edit it by hand.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(lockFile{Packages: entries}); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
