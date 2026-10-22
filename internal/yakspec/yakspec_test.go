package yakspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, src string) (*Spec, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.yakspec")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"no packages", "output: \"build\"\n", "packages names nothing to render"},
		{"another version", "spec: \"yakspec/v2\"\npackages: [{source: \"web\"}]\n", `"yakspec/v2" is not a version`},
		{"an unknown key", "outputs: \"build\"\npackages: [{source: \"web\"}]\n", "outputs is not a key of a spec file"},
		{"an unknown package key", "packages: [{source: \"web\", name: \"x\"}]\n", "packages[0].name is not a key of a package"},
		{"a package without a source", "packages: [{context: {}}]\n", "packages[0] has no source"},
		{"a wrong type", "strict: \"no\"\npackages: [{source: \"web\"}]\n", "strict must be true or false, found string"},
		{"an unknown format", "format: \"xml\"\npackages: [{source: \"web\"}]\n", `format unknown output format "xml"`},
		{"two documents", "packages: []\n---\npackages: []\n", "holds one document, not 2"},
		{"a context schema", "schema $$ = {}\npackages: [{source: \"web\"}]\n", "cannot declare a schema for it"},
		{"a library rule", "rules: {\"lib/x.yakl\": \"x.yaml\"}\npackages: [{source: \"web\"}]\n", "is a library"},
		{"a rule naming no template", "rules: {\"README.md\": \"x.yaml\"}\npackages: [{source: \"web\"}]\n", "names no template"},
		{"a rule climbing out", "rules: {\"app.yak\": \"../app.yaml\"}\npackages: [{source: \"web\"}]\n", "not a path beneath output"},
		{"a template sent to output itself", "rules: {\"app.yak\": \".\"}\npackages: [{source: \"web\"}]\n", "not a path beneath output"},
		{"a rule climbing out of the package", "rules: {\"../app.yak\": \"app.yaml\"}\npackages: [{source: \"web\"}]\n", "not a path beneath the package root"},
		{"a directory sent to a file", "rules: {\"deploy/\": \"k8s\"}\npackages: [{source: \"web\"}]\n", "must be sent to one"},
		{"a rule sent to a number", "rules: {\"app.yak\": 1}\npackages: [{source: \"web\"}]\n", "must be sent to a path or to null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestLoadRefusesASpecInsideAPackage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "yak.pkg"), []byte("package:\n  name: web\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "release.yakspec")
	if err := os.WriteFile(path, []byte("packages: [{source: \".\"}]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "cannot belong to one") {
		t.Errorf("error = %v, want the package refused", err)
	}
}

func TestLoadImportsAFilePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.yakl"), []byte("_source: \"web\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "release.yakspec")
	src := "import common = \"common.yakl\"\npackages: [{source: common._source}]\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.Packages[0].Source, filepath.Join(dir, "web"); got != want {
		t.Errorf("source = %q, want %q", got, want)
	}
}

func TestTarget(t *testing.T) {
	s, err := load(t, `rules:
  "./": "manifests/"
  "tests/": null
packages:
  - source: "web"
    rules:
      "deploy/": "k8s/"
      "deploy/app.toml.yak": "config/web.toml"
      "tests/keep.yak": "kept/"
  - source: "db"
    rules:
      "./": "databases/"
  - source: "cache"
    rules:
      "tests/": "tests/"
`)
	if err != nil {
		t.Fatal(err)
	}
	web, db, cache := s.Packages[0], s.Packages[1], s.Packages[2]
	for _, tc := range []struct {
		p    *Package
		rel  string
		want Target
	}{
		{web, "service.yak", Target{Path: "manifests", Rest: "service.yak"}},
		{web, "deploy/app.yak", Target{Path: "k8s", Rest: "app.yak"}},
		{web, "deploy/app.toml.yak", Target{Path: "config/web.toml", File: true}},
		{web, "tests/smoke.yak", Target{Drop: true}},
		{web, "tests/keep.yak", Target{Path: "kept", Rest: "keep.yak"}},
		{db, "db.yak", Target{Path: "databases", Rest: "db.yak"}},
		{db, "tests/db.yak", Target{Drop: true}},
		{cache, "tests/cache.yak", Target{Path: "tests", Rest: "cache.yak"}},
	} {
		if got := tc.p.Target("pkg", tc.rel); got != tc.want {
			t.Errorf("%s: Target(%s) = %+v, want %+v", tc.p.label, tc.rel, got, tc.want)
		}
	}
	if err := s.CheckRules(); err != nil {
		t.Errorf("CheckRules: %v", err)
	}
}

func TestCheckRulesRefusesARuleThatCoversNothing(t *testing.T) {
	s, err := load(t, "rules: {\"docs/\": null}\npackages:\n  - source: \"web\"\n    rules: {\"app.yak\": \"a.yaml\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	s.Packages[0].Target("web", "app.yak")
	if err := s.CheckRules(); err == nil || !strings.Contains(err.Error(), "rules: docs/ covers no template in any package") {
		t.Errorf("error = %v, want docs/ reported", err)
	}
}
