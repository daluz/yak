package module_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/module"
)

// tree writes files under a fresh directory and returns it.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolve(t *testing.T) {
	dir := tree(t, map[string]string{
		"yak.mod":          "modules:\n  a: one\n  a.b: two\n  m: one/m.yak\n",
		"one/b/c.yak":      "",
		"one/m.yak":        "",
		"two/c.yak":        "",
		"app/main.yak":     "",
		"app/local/x.yak":  "",
		"app/sub/deep.yak": "",
	})
	from := filepath.Join(dir, "app", "main.yak")
	tests := []struct {
		path string
		want string
	}{
		{"a.b.c", "two/c.yak"},
		{"m", "one/m.yak"},
		{"local.x", "app/local/x.yak"},
		{"sub.deep", "app/sub/deep.yak"},
	}
	for _, tc := range tests {
		got, err := module.Resolve(from, strings.Split(tc.path, "."))
		if err != nil {
			t.Errorf("Resolve(%q) returned error: %v", tc.path, err)
			continue
		}
		if want := filepath.Join(dir, tc.want); got != want {
			t.Errorf("Resolve(%q) = %s, want %s", tc.path, got, want)
		}
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name string
		mod  string
		path string
		want string
	}{
		{"inside a mapped module", "modules:\n  m: m.yak\n", "m.x", "is not inside a package"},
		{"a mapped package", "modules:\n  p: lib\n", "p", "maps \"p\" to the package"},
		{"a mapped module that is missing", "modules:\n  m: gone.yak\n", "m", "which does not exist"},
		{"mapping the standard library", "modules:\n  std.x: lib\n", "q", "reserved for the standard library"},
		{"a key that is not a path", "modules:\n  a-b: lib\n", "q", "is not a module path"},
		{"an unknown field", "packages: {}\n", "q", "field packages not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tree(t, map[string]string{"yak.mod": tc.mod, "m.yak": "", "lib/x.yak": ""})
			_, err := module.Resolve(filepath.Join(dir, "main.yak"), strings.Split(tc.path, "."))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
