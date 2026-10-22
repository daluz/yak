package yakspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/eval"
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
		{"an unknown format", "format: \"xml\"\npackages: [{source: \"web\"}]\n", "format"},
		{"two documents", "packages: []\n---\npackages: []\n", "holds one document, not 2"},
		{"a context schema", "schema $$ = {}\npackages: [{source: \"web\"}]\n", "cannot declare a schema for it"},
		{"a library rule", "rules: {\"lib/x.yakl\": \"x.yaml\"}\npackages: [{source: \"web\"}]\n", "is a library"},
		{"a rule naming no template", "rules: {\"README.md\": \"x.yaml\"}\npackages: [{source: \"web\"}]\n", "names no template"},
		{"a rule climbing out", "rules: {\"app.yak\": \"../app.yaml\"}\npackages: [{source: \"web\"}]\n", "not a path beneath output"},
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

func TestLoadIsNotStrict(t *testing.T) {
	t.Setenv("YAKSPEC_TEST", "eu")
	s, err := load(t, "import std.env\ncontext:\n  region: env.get(\"YAKSPEC_TEST\")\n  strict: $yak.strict\npackages: [{source: \"web\"}]\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.NonStrict {
		t.Error("the packages render out of strict mode, want strict by default")
	}
	ctx := s.Packages[0].Context.(*eval.Object)
	for name, want := range map[string]eval.Value{"region": eval.String("eu"), "strict": eval.Bool(false)} {
		f, ok := ctx.Lookup(name)
		if !ok {
			t.Fatalf("context has no %s", name)
		}
		if got, _ := f.Value.Value(); got != want {
			t.Errorf("context.%s = %v, want %v", name, got, want)
		}
	}
}

func TestLoadMergesThePackageContext(t *testing.T) {
	s, err := load(t, "context: {env: \"dev\", replicas: 2}\npackages:\n  - source: \"web\"\n    context: {replicas: 4}\n  - source: \"db\"\n")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []eval.Value{eval.Int(4), eval.Int(2)} {
		ctx := s.Packages[i].Context.(*eval.Object)
		env, _ := ctx.Lookup("env")
		replicas, _ := ctx.Lookup("replicas")
		if got, _ := replicas.Value.Value(); got != want {
			t.Errorf("packages[%d] replicas = %v, want %v", i, got, want)
		}
		if got, _ := env.Value.Value(); got != eval.String("dev") {
			t.Errorf("packages[%d] env = %v, want dev", i, got)
		}
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
		p         *Package
		rel       string
		want      Target
		wantDrops bool
	}{
		{web, "service.yak", Target{Path: "manifests", Rest: "service.yak"}, false},
		{web, "deploy/app.yak", Target{Path: "k8s", Rest: "app.yak"}, false},
		{web, "deploy/app.toml.yak", Target{Path: "config/web.toml", File: true}, false},
		{web, "tests/smoke.yak", Target{Drop: true}, true},
		{web, "tests/keep.yak", Target{Path: "kept", Rest: "keep.yak"}, false},
		{db, "db.yak", Target{Path: "databases", Rest: "db.yak"}, false},
		{db, "tests/db.yak", Target{Drop: true}, true},
		{cache, "tests/cache.yak", Target{Path: "tests", Rest: "cache.yak"}, false},
	} {
		if got := tc.p.Target("pkg", tc.rel); got != tc.want {
			t.Errorf("%s: Target(%s) = %+v, want %+v", tc.p.label, tc.rel, got, tc.want)
		}
	}
	if err := s.CheckRules(); err != nil {
		t.Errorf("CheckRules: %v", err)
	}
}

func TestTargetDefaultsToThePackageName(t *testing.T) {
	s, err := load(t, "packages: [{source: \"web\"}]\n")
	if err != nil {
		t.Fatal(err)
	}
	want := Target{Path: "web", Rest: "deploy/app.yak"}
	if got := s.Packages[0].Target("web", "deploy/app.yak"); got != want {
		t.Errorf("Target = %+v, want %+v", got, want)
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
