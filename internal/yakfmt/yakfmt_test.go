package yakfmt_test

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/yakfmt"
)

var update = flag.Bool("update", false, "rewrite golden files instead of comparing against them")

// typeNames are the TypeNames each golden file is formatted with, by the
// suffix its name takes before ".want.yak".
var typeNames = map[string]yakfmt.TypeNames{
	"":       yakfmt.KeepTypeNames,
	".short": yakfmt.ShortTypeNames,
	".long":  yakfmt.LongTypeNames,
}

// TestFormatGolden formats every testdata/fmt/NAME.yak and compares the
// result with NAME.want.yak, and with NAME.short.want.yak and
// NAME.long.want.yak where they exist.
func TestFormatGolden(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "fmt")
	inputs, err := filepath.Glob(filepath.Join(dir, "*.yak"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range inputs {
		if strings.HasSuffix(path, ".want.yak") {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".yak")
		for suffix, names := range typeNames {
			golden := filepath.Join(dir, name+suffix+".want.yak")
			if _, err := os.Stat(golden); suffix != "" && err != nil {
				continue
			}
			t.Run(name+suffix, func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				got, err := yakfmt.Source(path, src, names)
				if err != nil {
					t.Fatal(err)
				}
				if *update {
					if err := os.WriteFile(golden, got, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != string(want) {
					t.Errorf("formatting %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
				}
			})
		}
	}
}

// TestFormatEveryFixture formats every template and library under testdata
// that parses, with each of the TypeNames, each of which has to format
// without changing what it means and then stay as it is.
func TestFormatEveryFixture(t *testing.T) {
	for _, names := range typeNames {
		formatEveryFixture(t, names)
	}
}

func formatEveryFixture(t *testing.T, names yakfmt.TypeNames) {
	err := filepath.WalkDir(filepath.Join("..", "..", "testdata"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(path, ".yak") || strings.HasSuffix(path, ".yakl")) {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := parser.Parse(path, src); err != nil {
			return nil
		}
		once, err := yakfmt.Source(path, src, names)
		if err != nil {
			t.Error(err)
			return nil
		}
		twice, err := yakfmt.Source(path, once, names)
		if err != nil {
			t.Error(err)
			return nil
		}
		if string(once) != string(twice) {
			t.Errorf("%s formats differently the second time:\n%s", path, twice)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFormatReportsSyntaxErrors(t *testing.T) {
	_, err := yakfmt.Source("bad.yak", []byte("name: \"web\"\nport: [80\n"), yakfmt.KeepTypeNames)
	if err == nil || !strings.Contains(err.Error(), `bad.yak:2:7: unterminated flow sequence`) {
		t.Errorf("error = %v, want the parse error", err)
	}
}
