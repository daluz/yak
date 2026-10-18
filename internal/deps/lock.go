package deps

import (
	"bytes"
	"errors"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/module"
)

// writeLock writes entries to path, or removes path when there are none.
func writeLock(path string, entries map[string]module.LockEntry) error {
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
	if err := enc.Encode(module.Lock{Packages: entries}); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
