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

// TestFormatGolden formats every testdata/fmt/NAME.yak and compares the
// result with NAME.want.yak, which has to format to itself.
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
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := yakfmt.Source(path, src)
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join(dir, name+".want.yak")
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
			again, err := yakfmt.Source(golden, want)
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != string(want) {
				t.Errorf("%s does not format to itself:\n%s", golden, again)
			}
		})
	}
}

// TestFormatEveryFixture formats every template and library under testdata
// that parses, each of which has to format without changing what it means
// and then stay as it is.
func TestFormatEveryFixture(t *testing.T) {
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
		once, err := yakfmt.Source(path, src)
		if err != nil {
			t.Error(err)
			return nil
		}
		twice, err := yakfmt.Source(path, once)
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
	_, err := yakfmt.Source("bad.yak", []byte("name: \"web\"\nport: [80\n"))
	if err == nil || !strings.Contains(err.Error(), `bad.yak:2:7: unterminated flow sequence`) {
		t.Errorf("error = %v, want the parse error", err)
	}
}
