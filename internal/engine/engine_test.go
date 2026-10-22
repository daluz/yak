package engine_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/module"
	"github.com/daluz/yak/internal/render"
)

var update = flag.Bool("update", false, "rewrite golden files instead of comparing against them")

// testdataDir resolves a directory of fixtures at the repository root.
func testdataDir(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

// TestTemplateGolden renders every testdata/template/*.yak file against its
// sibling context files and compares the result with its goldens.
//
// Context files are named NAME.ctx*.yaml or NAME.ctx*.json and are passed in
// sorted order, so deployment.ctx1.yaml is merged before deployment.ctx2.yaml.
//
// Every template has a NAME.want.yaml. To cover another output format, add
// a NAME.want.FORMAT file beside it, or a NAME.want.FORMAT.err holding the
// diagnostic when the format cannot hold what the template produces.
func TestTemplateGolden(t *testing.T) {
	dir := testdataDir("template")
	templates, err := filepath.Glob(filepath.Join(dir, "*.yak"))
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatalf("no templates found in %s", dir)
	}

	for _, path := range templates {
		name := strings.TrimSuffix(filepath.Base(path), module.Ext)
		t.Run(name, func(t *testing.T) {
			contexts, err := filepath.Glob(filepath.Join(dir, name+".ctx*"))
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(contexts)

			for _, c := range goldenCases(t, dir, name) {
				t.Run(c.name(), func(t *testing.T) {
					var buf bytes.Buffer
					err := engine.Template(engine.TemplateRequest{
						Path:         path,
						ContextPaths: contexts,
						Out:          &buf,
						Options:      render.Options{Format: c.format},
					})
					if c.wantErr {
						if err == nil {
							t.Fatalf("rendering %s as %s succeeded, want an error", path, c.format)
						}
						compareGolden(t, c.golden, err.Error()+"\n")
						return
					}
					if err != nil {
						t.Fatalf("rendering %s as %s: %v", path, c.format, err)
					}
					requireValid(t, c.format, buf.Bytes())
					compareGolden(t, c.golden, buf.String())
				})
			}
		})
	}
}

// goldenCase is one template rendered in one format.
type goldenCase struct {
	format  render.Format
	golden  string
	wantErr bool
}

func (c goldenCase) name() string {
	if c.wantErr {
		return c.format.String() + "-error"
	}
	return c.format.String()
}

// goldenCases finds the goldens recorded for a template. The YAML one is
// always expected, so that a new fixture only needs `make update`.
func goldenCases(t *testing.T, dir, name string) []goldenCase {
	t.Helper()
	yamlGolden := filepath.Join(dir, name+".want.yaml")
	cases := []goldenCase{{format: render.FormatYAML, golden: yamlGolden}}

	prefix := name + ".want."
	paths, err := filepath.Glob(filepath.Join(dir, prefix+"*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if path == yamlGolden {
			continue
		}
		suffix := strings.TrimPrefix(filepath.Base(path), prefix)
		c := goldenCase{golden: path}
		if rest, ok := strings.CutSuffix(suffix, ".err"); ok {
			c.wantErr = true
			suffix = rest
		}
		format, err := render.ParseFormat(suffix)
		if err != nil {
			t.Fatalf("golden file %s: %v", path, err)
		}
		c.format = format
		cases = append(cases, c)
	}
	return cases
}

// TestTemplateErrors checks that every testdata/errors/*.yak file fails with
// the message recorded in its .want.err golden.
func TestTemplateErrors(t *testing.T) {
	dir := testdataDir("errors")
	templates, err := filepath.Glob(filepath.Join(dir, "*.yak"))
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatalf("no templates found in %s", dir)
	}

	for _, path := range templates {
		name := strings.TrimSuffix(filepath.Base(path), module.Ext)
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			err := engine.Template(engine.TemplateRequest{
				Path:    path,
				Out:     &buf,
				Options: render.DefaultOptions(),
			})
			if err == nil {
				t.Fatalf("rendering %s succeeded, want an error", path)
			}
			// Golden files record the message without the fixtures' directory
			// prefix, which modules the fixture imports share, so they stay
			// stable wherever the tests run from.
			got := strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), "")
			compareGolden(t, filepath.Join(dir, name+".want.err"), got+"\n")
		})
	}
}

// TestSchemaCheck checks that every testdata/schemas/*.yak file fails
// "yak schema check" with the message recorded in its .want.err golden, and
// that every template in testdata/template passes it.
func TestSchemaCheck(t *testing.T) {
	dir := testdataDir("schemas")
	templates, err := filepath.Glob(filepath.Join(dir, "*.yak"))
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatalf("no templates found in %s", dir)
	}
	for _, path := range templates {
		name := strings.TrimSuffix(filepath.Base(path), module.Ext)
		t.Run(name, func(t *testing.T) {
			err := engine.CheckSchemas(path)
			if err == nil {
				t.Fatalf("checking %s succeeded, want an error", path)
			}
			got := strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), "")
			compareGolden(t, filepath.Join(dir, name+".want.err"), got+"\n")
		})
	}

	passing, err := filepath.Glob(filepath.Join(testdataDir("template"), "*.yak"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range passing {
		if err := engine.CheckSchemas(path); err != nil {
			t.Errorf("checking %s: %v", path, err)
		}
	}
}

func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden file (run `go test ./... -update` to create it): %v", err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}

func TestTemplateReportsMissingFile(t *testing.T) {
	var buf bytes.Buffer
	err := engine.Template(engine.TemplateRequest{
		Path:    filepath.Join(t.TempDir(), "absent.yak"),
		Out:     &buf,
		Options: render.DefaultOptions(),
	})
	if err == nil || !strings.Contains(err.Error(), "reading template") {
		t.Errorf("error = %v, want a read failure", err)
	}
}

// renderFile writes src as a template at name beneath dir and renders it.
func renderFile(t *testing.T, dir, name, src string, nonStrict bool) (string, error) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := engine.Template(engine.TemplateRequest{Path: path, Out: &buf, Options: render.DefaultOptions(), NonStrict: nonStrict})
	return buf.String(), err
}

func TestFilesReadAnywhereWhenNotStrict(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"parent": "../outside.txt", "absolute": filepath.ToSlash(outside)} {
		t.Run(name, func(t *testing.T) {
			src := "import std.files\nv: files.read(\"" + path + "\")\n"
			got, err := renderFile(t, dir, "sub/t.yak", src, true)
			if err != nil {
				t.Fatal(err)
			}
			if got != "v: outside\n" {
				t.Errorf("got %q, want %q", got, "v: outside\n")
			}
		})
	}
}

func TestFilesStrictRefusesEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "outside.txt"), filepath.Join(dir, "sub", "link.txt")); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
	src := "import std.files\nv: files.read(\"link.txt\")\n"
	if _, err := renderFile(t, dir, "sub/t.yak", src, false); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("strict error = %v, want the link to be refused", err)
	}
	if got, err := renderFile(t, dir, "sub/t.yak", src, true); err != nil || got != "v: outside\n" {
		t.Errorf("non-strict got %q, %v, want the link followed", got, err)
	}
}

func TestRandomWhenNotStrict(t *testing.T) {
	src := `import { std.random, std.strings }
same: random.randint(4, 4)
between: random.randint(1, 3) in [1, 2, 3]
fraction: random.rand() < 1
only: random.choice(["a"])
picked: random.choice([1, 2, 3]) in [1, 2, 3]
shuffled: random.shuffle([7])
let id = random.uuid()
uuid: size(id) == 36 and strings.startsWith(strings.split(id, "-")[2], "4")
`
	got, err := renderFile(t, t.TempDir(), "t.yak", src, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "same: 4\nbetween: true\nfraction: true\nonly: a\npicked: true\nshuffled:\n  - 7\nuuid: true\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEnvWhenNotStrict(t *testing.T) {
	t.Setenv("YAK_TEST_SET", "shop")
	t.Setenv("YAK_TEST_EMPTY", "")
	src := `import std.env
set: env.get("YAK_TEST_SET")
empty: env.get("YAK_TEST_EMPTY", "fallback")
unset: env.get("YAK_TEST_UNSET")
fallback: env.get("YAK_TEST_UNSET", 8080)
`
	got, err := renderFile(t, t.TempDir(), "t.yak", src, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "set: shop\nempty: \"\"\nunset: null\nfallback: 8080\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestImportFilePath(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"lib/my-base.yak":       "x: 1\n",
		"lib/v1.2/helpers.yakl": "_y: 2\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	src := "import {\n  base = \"lib/my-base.yak\"\n  helpers = \"lib/v1.2/helpers.yakl\"\n}\nx: base.x\ny: helpers._y\n"
	got, err := renderFile(t, dir, "t.yak", src, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := "x: 1\ny: 2\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImportFilePathRefuses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.yak"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, want string }{
		{"a parent", "../base.yak", "is not a path beneath"},
		{"an absolute path", filepath.ToSlash(filepath.Join(dir, "base.yak")), "is not a path beneath"},
		{"no extension", "base", "names no module"},
		{"a missing file", "absent.yak", "does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "import base = \"" + tc.path + "\"\nx: base.x\n"
			if _, err := renderFile(t, dir, "sub/t.yak", src, true); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestImportFilePathRefusedInsideAPackage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "yak.pkg"), []byte("package:\n  name: web\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.yak"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := "import base = \"base.yak\"\nx: base.x\n"
	if _, err := renderFile(t, dir, "t.yak", src, true); err == nil || !strings.Contains(err.Error(), "inside the package web") {
		t.Errorf("error = %v, want the package refused", err)
	}
}

func TestTemplateWritesNothingOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yak")
	if err := os.WriteFile(path, []byte("a: 1\nb: .missing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := engine.Template(engine.TemplateRequest{Path: path, Out: &buf, Options: render.DefaultOptions()}); err == nil {
		t.Fatal("expected an error")
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %q before failing, want nothing", buf.String())
	}
}
