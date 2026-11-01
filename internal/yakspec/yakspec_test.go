package yakspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/module"
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
		{"the rules of version 1 at the top", "rules: {\"/\": \"out/\"}\npackages: [{source: \"web\"}]\n", "rules is not a key of a spec file"},
		{"the rules of version 1 in a package", "packages: [{source: \"web\", rules: {}}]\n", "packages[0].rules is not a key of a package"},
		{"an unknown render key", "render: {includes: []}\npackages: [{source: \"web\"}]\n", "render.includes is not a key of render; it takes rules"},
		{"a render that is no mapping", "packages: [{source: \"web\", render: true}]\n", "packages[0].render must be a mapping"},
		{"a library rule", "render: {rules: {\"lib/x.yakl\": \"x.yaml\"}}\npackages: [{source: \"web\"}]\n", "is a library"},
		{"a rule climbing out", "render: {rules: {\"app.yak\": \"../app.yaml\"}}\npackages: [{source: \"web\"}]\n", "not a path beneath the output"},
		{"a template sent to output itself", "render: {rules: {\"app.yak\": \".\"}}\npackages: [{source: \"web\"}]\n", "not a path beneath the output"},
		{"the package written ./", "render: {rules: {\"./\": \"k8s/\"}}\npackages: [{source: \"web\"}]\n", `write "/" for the whole package`},
		{"the output written ./", "render: {rules: {\"deploy/\": \"./\"}}\npackages: [{source: \"web\"}]\n", `write "/" for the root of the output`},
		{"a rule climbing out of the package", "render: {rules: {\"../app.yak\": \"app.yaml\"}}\npackages: [{source: \"web\"}]\n", "not a path beneath the package root"},
		{"a directory sent to a file", "render: {rules: {\"deploy/\": \"k8s\"}}\npackages: [{source: \"web\"}]\n", "must be sent to one"},
		{"a rule sent to a number", "render: {rules: {\"app.yak\": 1}}\npackages: [{source: \"web\"}]\n", "must be sent to a path or to null"},
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

func TestRules(t *testing.T) {
	s, err := load(t, `render:
  rules:
    "/": "manifests/"
    "tests/": null
packages:
  - source: "web"
    render:
      rules:
        "/deploy/": "k8s/"
        "deploy/app.toml.yak": "/config/web.toml"
        "tests/keep.yak": "kept/"
  - source: "db"
    render:
      rules:
        "/": "databases/"
  - source: "cache"
    render:
      rules:
        "tests/": "tests/"
  - source: "flat"
    render:
      rules:
        "/": "/"
`)
	if err != nil {
		t.Fatal(err)
	}
	web, db, cache, flat := s.Packages[0], s.Packages[1], s.Packages[2], s.Packages[3]
	for _, tc := range []struct {
		p    *Package
		rel  string
		want module.Target
	}{
		{web, "service.yak", module.Target{Path: "manifests", Rest: "service.yak"}},
		{web, "deploy/app.yak", module.Target{Path: "k8s", Rest: "app.yak"}},
		{web, "deploy/app.toml.yak", module.Target{Path: "config/web.toml", File: true}},
		{web, "tests/smoke.yak", module.Target{Drop: true}},
		{web, "tests/keep.yak", module.Target{Path: "kept", Rest: "keep.yak"}},
		{db, "db.yak", module.Target{Path: "databases", Rest: "db.yak"}},
		{db, "tests/db.yak", module.Target{Drop: true}},
		{cache, "tests/cache.yak", module.Target{Path: "tests", Rest: "cache.yak"}},
		{flat, "deploy/flat.yak", module.Target{Path: "", Rest: "deploy/flat.yak"}},
	} {
		if got, _ := module.Place(tc.p.Rules(), tc.rel); got != tc.want {
			t.Errorf("%s: Place(%s) = %+v, want %+v", tc.p.label, tc.rel, got, tc.want)
		}
	}
	if err := s.CheckRules(); err != nil {
		t.Errorf("CheckRules: %v", err)
	}
}

func TestCheckRulesRefusesARuleThatCoversNothing(t *testing.T) {
	s, err := load(t, "render: {rules: {\"docs/\": null}}\npackages:\n  - source: \"web\"\n    render: {rules: {\"app.yak\": \"a.yaml\"}}\n")
	if err != nil {
		t.Fatal(err)
	}
	module.Place(s.Packages[0].Rules(), "app.yak")
	if err := s.CheckRules(); err == nil || !strings.Contains(err.Error(), "render.rules: docs/ covers no template or included file in any package") {
		t.Errorf("error = %v, want docs/ reported", err)
	}
}
