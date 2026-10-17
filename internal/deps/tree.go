package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// copyTree copies the repository checked out at from to to, leaving out its
// .git directory.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			return filepath.SkipDir
		}
		target := filepath.Join(to, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, src, 0o644)
		default:
			return fmt.Errorf("%s is not a regular file, which a vendored package cannot hold", filepath.ToSlash(rel))
		}
	})
}

// hashDir hashes the names and contents of every file beneath dir.
func hashDir(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(src)
		fmt.Fprintf(h, "%s\x00%s\n", filepath.ToSlash(rel), hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// prune removes from the vendor directory every repository but those keep
// names, and the vendor directory itself when keep names none. It leaves a
// .gitignore in the directory, so that it is never committed.
func prune(vendor string, keep map[string]lockEntry) error {
	if len(keep) == 0 {
		return os.RemoveAll(vendor)
	}
	if err := os.WriteFile(filepath.Join(vendor, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		return err
	}
	return filepath.WalkDir(vendor, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == vendor {
			return err
		}
		rel, err := filepath.Rel(vendor, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".gitignore" {
			return nil
		}
		if _, ok := keep[rel]; ok {
			return filepath.SkipDir
		}
		if d.IsDir() {
			for key := range keep {
				if strings.HasPrefix(key, rel+"/") {
					return nil
				}
			}
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}
