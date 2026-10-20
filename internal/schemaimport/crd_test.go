package schemaimport_test

import (
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/schemaimport"
)

var update = flag.Bool("update", false, "rewrite golden files instead of comparing against them")

// TestCRDsGolden imports every testdata/import/*.yaml file. The library it
// answers is recorded in NAME.want.yakl and has to pass "yak schema check",
// and its warnings, then its error, in NAME.want.err; a golden that would be
// empty is not written.
func TestCRDsGolden(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "import")
	inputs, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no CRDs found in %s", dir)
	}
	for _, path := range inputs {
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lib, warnings, err := schemaimport.CRDs([]schemaimport.File{{Name: name + ".yaml", Data: data}})
			diagnostics := strings.Join(warnings, "\n")
			if err != nil {
				diagnostics = strings.TrimPrefix(diagnostics+"\n"+err.Error(), "\n")
			}
			if diagnostics != "" {
				diagnostics += "\n"
			}
			golden := filepath.Join(dir, name+".want.yakl")
			compareGolden(t, golden, string(lib))
			compareGolden(t, filepath.Join(dir, name+".want.err"), diagnostics)
			if err == nil {
				if err := engine.CheckSchemas(golden); err != nil {
					t.Errorf("checking the library: %v", err)
				}
			}
		})
	}
}

// compareGolden compares got with golden, where an empty got stands for a
// golden that does not exist.
func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if got == "" {
			if err := os.Remove(golden); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}
