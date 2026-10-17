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
		"yak.pkg":           "package:\n  name: app\ndependencies:\n  a: one\n  a.b: two\n  m: one/m.yak\n  l: one/l.yakl\n",
		"one/yak.pkg":       "package:\n  name: one\n",
		"one/b/c.yak":       "",
		"one/m.yak":         "",
		"one/l.yakl":        "",
		"two/yak.pkg":       "package:\n  name: two\n",
		"two/c.yak":         "",
		"app/main.yak":      "",
		"app/local/x.yak":   "",
		"app/sub/deep.yakl": "",
		"top.yak":           "",
	})
	from := filepath.Join(dir, "app", "main.yak")
	tests := []struct {
		path string
		want string
	}{
		{"a.b.c", "two/c.yak"},
		{"m", "one/m.yak"},
		{"l", "one/l.yakl"},
		{"app.local.x", "app/local/x.yak"},
		{"app.sub.deep", "app/sub/deep.yakl"},
		{"top", "top.yak"},
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

// TestResolveOutsideAPackage checks that a file in no package imports from
// its own directory.
func TestResolveOutsideAPackage(t *testing.T) {
	dir := tree(t, map[string]string{"app/main.yak": "", "app/lib/x.yak": ""})
	got, err := module.Resolve(filepath.Join(dir, "app", "main.yak"), []string{"lib", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "app", "lib", "x.yak"); got != want {
		t.Errorf("Resolve = %s, want %s", got, want)
	}
}

// TestResolveVendored checks that a git dependency resolves into the vendor
// directory of its package, and that a package vendored there finds what it
// depends on beside it.
func TestResolveVendored(t *testing.T) {
	const lib = "git+https://example.com/acme/lib.git//pkgs/lib@v1"
	const util = "git+ssh://git@example.com/acme/util.git"
	dir := tree(t, map[string]string{
		"yak.pkg": "package:\n  name: app\ndependencies:\n  lib: " + lib + "\n",
		"app.yak": "",
		"vendor/example.com/acme/lib/pkgs/lib/yak.pkg": "package:\n  name: lib\ndependencies:\n  util: " + util + "\n",
		"vendor/example.com/acme/lib/pkgs/lib/x.yakl":  "",
		"vendor/example.com/acme/util/yak.pkg":         "package:\n  name: util\n",
		"vendor/example.com/acme/util/y.yak":           "",
	})
	got, err := module.Resolve(filepath.Join(dir, "app.yak"), []string{"lib", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "vendor/example.com/acme/lib/pkgs/lib/x.yakl"); got != want {
		t.Errorf("Resolve(lib.x) = %s, want %s", got, want)
	}
	got, err = module.Resolve(got, []string{"util", "y"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "vendor/example.com/acme/util/y.yak"); got != want {
		t.Errorf("Resolve(util.y) = %s, want %s", got, want)
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		target string
		want   module.Source
	}{
		{"git+https://github.com/acme/lib.git@v1.2.0",
			module.Source{Repo: "https://github.com/acme/lib.git", Ref: "v1.2.0", Key: "github.com/acme/lib"}},
		{"git+https://github.com/acme/mono.git//pkgs/web@feature/x",
			module.Source{Repo: "https://github.com/acme/mono.git", Dir: "pkgs/web", Ref: "feature/x", Key: "github.com/acme/mono"}},
		{"git+ssh://git@gitlab.com:22/group/sub/lib",
			module.Source{Repo: "ssh://git@gitlab.com:22/group/sub/lib", Key: "gitlab.com/group/sub/lib"}},
		{"git+file:///srv/git/lib.git",
			module.Source{Repo: "file:///srv/git/lib.git", Key: "srv/git/lib"}},
	}
	for _, tc := range tests {
		got, err := module.ParseSource(tc.target)
		if err != nil {
			t.Errorf("ParseSource(%q) returned error: %v", tc.target, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSource(%q) = %+v, want %+v", tc.target, got, tc.want)
		}
	}
	for target, want := range map[string]string{
		"https://github.com/acme/lib.git":       "written as git+https://",
		"git+ftp://example.com/lib.git":         "cannot fetch over \"ftp\"",
		"git+https://github.com/acme/lib.git@":  "nothing follows",
		"git+https://github.com/acme/lib//../x": "is not a directory inside the repository",
		"git+https://github.com/":               "names no repository",
		"oci://ghcr.io/acme/lib:1":              "OCI packages are not implemented yet",
	} {
		if _, err := module.ParseSource(target); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseSource(%q) error = %v, want it to contain %q", target, err, want)
		}
	}
}

func TestResolveErrors(t *testing.T) {
	const pkg = "package:\n  name: p\n"
	tests := []struct {
		name string
		pkg  string
		path string
		want string
	}{
		{"inside a mapped module", pkg + "dependencies:\n  m: m.yak\n", "m.x", "is not inside a package"},
		{"a mapped package", pkg + "dependencies:\n  p: lib\n", "p", "maps \"p\" to the package"},
		{"a mapped directory that is not a package", pkg + "dependencies:\n  p: plain\n", "p.x", "which is not a package"},
		{"a mapped module that is missing", pkg + "dependencies:\n  m: gone.yak\n", "m", "which does not exist"},
		{"mapping the standard library", pkg + "dependencies:\n  std.x: lib\n", "q", "reserved for the standard library"},
		{"a key that is not a path", pkg + "dependencies:\n  a-b: lib\n", "q", "is not a module path"},
		{"an OCI package", pkg + "dependencies:\n  r: oci://example.com/r:1\n", "r.x", "OCI packages are not implemented yet"},
		{"a git package that is not vendored", pkg + "dependencies:\n  r: git+https://example.com/r.git@v1\n", "r.x", "which is not vendored; run yak sync"},
		{"an unknown field", pkg + "modules: {}\n", "q", "field modules not found"},
		{"a package without a name", "dependencies: {}\n", "q", "the package has no name"},
		{"a name that is not an identifier", "package:\n  name: a-b\n", "q", "cannot name a package"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tree(t, map[string]string{
				"yak.pkg": tc.pkg, "m.yak": "", "lib/yak.pkg": pkg, "lib/x.yak": "", "plain/x.yak": "",
			})
			_, err := module.Resolve(filepath.Join(dir, "main.yak"), strings.Split(tc.path, "."))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
