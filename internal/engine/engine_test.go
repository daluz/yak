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
		name := strings.TrimSuffix(filepath.Base(path), engine.ExtTemplate)
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
		name := strings.TrimSuffix(filepath.Base(path), engine.ExtTemplate)
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
			// Golden files record the message without the fixture's directory
			// prefix so they stay stable wherever the tests run from.
			got := strings.ReplaceAll(err.Error(), path, filepath.Base(path))
			compareGolden(t, filepath.Join(dir, name+".want.err"), got+"\n")
		})
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

func TestTemplateRejectsLibraryFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared"+engine.ExtLibrary)
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := engine.Template(engine.TemplateRequest{Path: path, Out: &buf, Options: render.DefaultOptions()})
	if err == nil || !strings.Contains(err.Error(), "meant to be imported") {
		t.Errorf("error = %v, want a rejection of .libyak input", err)
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
