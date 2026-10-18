package module

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LockFile is the name of the file, beside yak.pkg, that pins the
// repositories a package's remote dependencies are vendored from. The vendor
// directory holds one of the same name, recording what was vendored.
const LockFile = "yak.lock"

// LockEntry pins one repository: a git repository at a commit, or an OCI
// repository at a manifest digest, with a hash of the files vendored from it.
type LockEntry struct {
	Source string `yaml:"source"`
	Ref    string `yaml:"ref,omitempty"`
	Commit string `yaml:"commit,omitempty"`
	Digest string `yaml:"digest,omitempty"`
	Hash   string `yaml:"hash"`
}

// Lock is a parsed lock file.
type Lock struct {
	// Packages is keyed by the repository's Source.Key.
	Packages map[string]LockEntry `yaml:"packages"`
}

// ReadLock reads the lock file at path, which is empty when there is none.
func ReadLock(path string) (map[string]LockEntry, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]LockEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var lock Lock
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	if err := dec.Decode(&lock); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if lock.Packages == nil {
		lock.Packages = map[string]LockEntry{}
	}
	return lock.Packages, nil
}

// checkVendored reports an error unless the vendor directory vendor holds
// src as the yak.lock beside it pins it, and pins it as target writes it.
func checkVendored(vendor string, src Source, target string) error {
	root := filepath.Dir(vendor)
	pinned, err := ReadLock(filepath.Join(root, LockFile))
	if err != nil {
		return err
	}
	p, ok := pinned[src.Key]
	if !ok || p.Source != src.Repo || p.Ref != src.Ref {
		return fmt.Errorf("%s pins no %s; run yak sync", filepath.Join(root, LockFile), target)
	}
	have, err := ReadLock(filepath.Join(vendor, LockFile))
	if err != nil {
		return err
	}
	if have[src.Key] != p {
		return fmt.Errorf("%s does not hold %s as %s pins it; run yak sync",
			filepath.Join(root, VendorDir), src.Key, filepath.Join(root, LockFile))
	}
	return nil
}
