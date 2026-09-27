package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/cli"
)

// run executes the CLI with the given arguments and returns its stdout.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := cli.NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func write(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTemplateWritesToStdout(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\nalias: .name\n")

	out, err := run(t, "template", tmpl)
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	want := "name: web\nalias: web\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestTemplateMergesContextFlags(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: $$.name\ntier: $$.tier\n")
	first := write(t, dir, "one.yaml", "name: base\ntier: base\n")
	second := write(t, dir, "two.yaml", "tier: override\n")

	out, err := run(t, "template", tmpl, "-c", first, "-c", second)
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	want := "name: base\ntier: override\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestTemplateAcceptsJSONContext(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: $$.name\ntier: $$.tier\n")
	yamlCtx := write(t, dir, "one.yaml", "name: base\ntier: base\n")
	jsonCtx := write(t, dir, "two.json", `{"tier": "override"}`)

	out, err := run(t, "template", tmpl, "-c", yamlCtx, "-c", jsonCtx)
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	want := "name: base\ntier: override\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestTemplateWritesToFile(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.yaml")

	out, err := run(t, "template", tmpl, "-o", dest)
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want it to be empty when -o is used", out)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name: web\n" {
		t.Errorf("file contents = %q, want %q", got, "name: web\n")
	}
}

func TestTemplateLeavesOutputFileAloneOnFailure(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "broken.yak", "a: .missing\n")
	dest := write(t, dir, "out.yaml", "previous contents\n")

	if _, err := run(t, "template", tmpl, "-o", dest); err == nil {
		t.Fatal("expected an error")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous contents\n" {
		t.Errorf("output file was modified: %q", got)
	}
}

func TestTemplateRequiresOneArgument(t *testing.T) {
	if _, err := run(t, "template"); err == nil {
		t.Error("expected an error when no template is given")
	}
}

func TestTemplateErrorMentionsPosition(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "broken.yak", "a: 1\nb: yes\n")

	_, err := run(t, "template", tmpl)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "broken.yak:2:4") {
		t.Errorf("error = %q, want it to point at broken.yak:2:4", err)
	}
}

func TestTemplateFormatFlag(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	for _, tc := range []struct {
		format string
		want   string
	}{
		{"json", "{\n  \"name\": \"web\"\n}\n"},
		{"jsonl", "{\"name\":\"web\"}\n"},
		{"toml", "name = \"web\"\n"},
		{"jwcc", "{\n  \"name\": \"web\",\n}\n"},
		// jsoncc, hujson and json5 are other names for jwcc.
		{"jsoncc", "{\n  \"name\": \"web\",\n}\n"},
		{"hujson", "{\n  \"name\": \"web\",\n}\n"},
		{"json5", "{\n  \"name\": \"web\",\n}\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			out, err := run(t, "template", tmpl, "-f", tc.format)
			if err != nil {
				t.Fatalf("template returned error: %v", err)
			}
			if out != tc.want {
				t.Errorf("stdout = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestTemplateDropsNullDocuments(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "local unused = 1\n---\nname: \"web\"\n")

	out, err := run(t, "template", tmpl)
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	if out != "name: web\n" {
		t.Errorf("stdout = %q, want the null document to be left out", out)
	}
}

func TestTemplateKeepsNullDocumentsOnRequest(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "local unused = 1\n---\nname: \"web\"\n")

	out, err := run(t, "template", tmpl, "--keep-null-documents")
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	if out != "null\n---\nname: web\n" {
		t.Errorf("stdout = %q, want the null document to be kept", out)
	}
}

func TestTemplateRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	_, err := run(t, "template", tmpl, "-f", "xml")
	if err == nil || !strings.Contains(err.Error(), `unknown output format "xml"`) {
		t.Errorf("error = %v, want a complaint about the format name", err)
	}
}

func TestTemplateInfersFormatFromOutputFile(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.toml")

	if _, err := run(t, "template", tmpl, "-o", dest); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name = \"web\"\n" {
		t.Errorf("file contents = %q, want TOML", got)
	}
}

func TestTemplateFormatFlagBeatsOutputExtension(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.toml")

	if _, err := run(t, "template", tmpl, "-o", dest, "-f", "json"); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\n  \"name\": \"web\"\n}\n" {
		t.Errorf("file contents = %q, want JSON", got)
	}
}

func TestTemplateKeepsYAMLForAnUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.txt")

	if _, err := run(t, "template", tmpl, "-o", dest); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name: web\n" {
		t.Errorf("file contents = %q, want YAML", got)
	}
}

func TestTemplateInfersFormatFromTemplateName(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")

	out, err := run(t, "template", tmpl)
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	if out != "name = \"web\"\n" {
		t.Errorf("stdout = %q, want TOML", out)
	}
}

func TestTemplateOutputExtensionBeatsTemplateName(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.json")

	if _, err := run(t, "template", tmpl, "-o", dest); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\n  \"name\": \"web\"\n}\n" {
		t.Errorf("file contents = %q, want JSON", got)
	}
}

func TestTemplateNameHoldsForAnUnknownOutputExtension(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.txt")

	if _, err := run(t, "template", tmpl, "-o", dest); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name = \"web\"\n" {
		t.Errorf("file contents = %q, want TOML", got)
	}
}

func TestTemplateAutomaticOutput(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")

	out, err := run(t, "template", tmpl, "-O")
	if err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want it to be empty when -O is used", out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "app.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name = \"web\"\n" {
		t.Errorf("file contents = %q, want TOML", got)
	}
}

func TestTemplateAutomaticOutputNamesTheFormat(t *testing.T) {
	for _, tc := range []struct {
		template string
		args     []string
		want     string
	}{
		{"app.yak", nil, "app.yaml"},
		{"app.toml.yak", nil, "app.toml"},
		{"app.yml.yak", nil, "app.yaml"},
		{"app.toml.yak", []string{"-f", "json"}, "app.json"},
	} {
		t.Run(tc.template+strings.Join(tc.args, " "), func(t *testing.T) {
			dir := t.TempDir()
			tmpl := write(t, dir, tc.template, "name: \"web\"\n")
			args := append([]string{"template", tmpl, "-O"}, tc.args...)

			if _, err := run(t, args...); err != nil {
				t.Fatalf("template returned error: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.want)); err != nil {
				t.Errorf("want %s to have been written: %v", tc.want, err)
			}
		})
	}
}

func TestTemplateAutomaticOutputRejectsStandardInput(t *testing.T) {
	_, err := run(t, "template", "-", "-O")
	if err == nil || !strings.Contains(err.Error(), "--automatic-output needs a .yak template") {
		t.Errorf("error = %v, want a complaint about the template name", err)
	}
}

func TestTemplateRejectsOutputAndAutomaticOutputTogether(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	if _, err := run(t, "template", tmpl, "-O", "-o", filepath.Join(dir, "out.yaml")); err == nil {
		t.Error("expected an error when both -o and -O are given")
	}
}

func TestTemplateOutputDirCreatesTheDirectory(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")
	outDir := filepath.Join(dir, "build", "nested")

	if _, err := run(t, "template", tmpl, "-O", "--output-dir", outDir); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "app.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name = \"web\"\n" {
		t.Errorf("file contents = %q, want TOML", got)
	}
}

func TestTemplateOutputDirJoinsTheOutputPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	write(t, dir, "app.yak", "name: \"web\"\n")

	if _, err := run(t, "template", "app.yak", "-o", "sub/out.json", "--output-dir", "build"); err != nil {
		t.Fatalf("template returned error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "build", "sub", "out.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\n  \"name\": \"web\"\n}\n" {
		t.Errorf("file contents = %q, want JSON", got)
	}
}

func TestTemplateOutputDirRejectsAnAbsoluteOutput(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	_, err := run(t, "template", tmpl, "-o", filepath.Join(dir, "out.yaml"), "--output-dir", dir)
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error = %v, want a complaint about the absolute output path", err)
	}
}

func TestTemplateOutputDirNeedsAnOutputFile(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	_, err := run(t, "template", tmpl, "--output-dir", dir)
	if err == nil || !strings.Contains(err.Error(), "--output-dir needs") {
		t.Errorf("error = %v, want a complaint about the missing output file", err)
	}
}

func TestBuildAndValidateAreNotRegisteredYet(t *testing.T) {
	for _, name := range []string{"build", "validate"} {
		if _, err := run(t, name); err == nil {
			t.Errorf("%q succeeded, but it is not implemented yet", name)
		}
	}
}
