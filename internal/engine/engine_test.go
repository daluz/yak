package engine_test

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/render"
)

var update = flag.Bool("update", false, "rewrite golden files instead of comparing against them")

// testdataDir resolves a directory of fixtures at the repository root.
func testdataDir(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

// TestTemplateGolden renders every testdata/template/*.yak file against its
// sibling context files and compares the result with the .want.yaml golden.
//
// Context files are named NAME.ctx*.yaml or NAME.ctx*.json and are passed in
// sorted order, so deployment.ctx1.yaml is merged before deployment.ctx2.yaml.
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

			var buf bytes.Buffer
			err = engine.Template(engine.TemplateRequest{
				Path:         path,
				ContextPaths: contexts,
				Out:          &buf,
				Options:      render.DefaultOptions(),
			})
			if err != nil {
				t.Fatalf("rendering %s: %v", path, err)
			}
			requireValidYAML(t, buf.Bytes())
			compareGolden(t, filepath.Join(dir, name+".want.yaml"), buf.String())
		})
	}
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

// requireValidYAML reads the rendered stream back. Comments are written into
// the output, and one placed badly would change the shape of a document
// rather than just look wrong.
func requireValidYAML(t *testing.T, out []byte) {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("rendered output is not valid YAML: %v\n%s", err, out)
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
