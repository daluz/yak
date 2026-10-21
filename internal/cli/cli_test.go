package cli_test

import (
	"bytes"
	"os"
	"os/exec"
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderWritesToStdout(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\nalias: .name\n")

	out, err := run(t, "render", tmpl)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	want := "name: web\nalias: web\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRenderMergesContextFlags(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: $$.name\ntier: $$.tier\n")
	first := write(t, dir, "one.yaml", "name: base\ntier: base\n")
	second := write(t, dir, "two.yaml", "tier: override\n")

	out, err := run(t, "render", tmpl, "-c", first, "-c", second)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	want := "name: base\ntier: override\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRenderAcceptsJSONContext(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: $$.name\ntier: $$.tier\n")
	yamlCtx := write(t, dir, "one.yaml", "name: base\ntier: base\n")
	jsonCtx := write(t, dir, "two.json", `{"tier": "override"}`)

	out, err := run(t, "render", tmpl, "-c", yamlCtx, "-c", jsonCtx)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	want := "name: base\ntier: override\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRenderIsStrictByDefault(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "import std.random\nn: random.randint(1, 1)\n")

	if _, err := run(t, "render", tmpl); err == nil || !strings.Contains(err.Error(), "--strict=false") {
		t.Errorf("error = %v, want strict mode to refuse std.random", err)
	}
	out, err := run(t, "render", tmpl, "--strict=false")
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if out != "n: 1\n" {
		t.Errorf("stdout = %q, want %q", out, "n: 1\n")
	}
}

func TestRenderSkipChecksKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "schema $$ =\n  env: \"dev\" | \"prod\"\n"+
		"schema Port =\n  name: string\n  number: integer if (n) => n > 0\n  protocol: string = \"TCP\"\n"+
		"port: !Port {number: -1, extra: true}\nenv: $$.env\n")
	ctx := write(t, dir, "qa.yaml", "env: qa\n")

	if _, err := run(t, "render", tmpl, "-c", ctx); err == nil {
		t.Fatal("render succeeded, want the context to be refused")
	}
	out, err := run(t, "render", tmpl, "-c", ctx, "--skip-schema-checks")
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	want := "port:\n  number: -1\n  extra: true\n  protocol: TCP\nenv: qa\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestSchemaCheckReadsLibrariesBeneathADirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.yak", "schema Port = {name: string}\nport: !Port {name: \"http\"}\n")
	write(t, dir, "lib/k8s.yakl", "schema Deployment =\n  image: strng\n")

	if _, err := run(t, "schema", "check", filepath.Join(dir, "app.yak")); err != nil {
		t.Fatalf("checking the template returned error: %v", err)
	}
	_, err := run(t, "schema", "check", dir)
	if err == nil || !strings.Contains(err.Error(), `k8s.yakl:2:10: no schema or type "strng" is in scope`) {
		t.Errorf("error = %v, want the library's unknown type", err)
	}
}

func TestFmtWritesToStdout(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "a:\n    b: [ 1,2 ]\n")

	out, err := run(t, "fmt", tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a:\n  b: [1, 2]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestFmtListsAndRewritesADirectory(t *testing.T) {
	dir := t.TempDir()
	messy := write(t, dir, "app.yak", "a:    1\n")
	tidy := write(t, dir, "lib/tidy.yakl", "a: 1\n")
	write(t, dir, "bad.yak", "a: [1\n")

	out, err := run(t, "fmt", "-l", "-w", dir)
	if err == nil || !strings.Contains(err.Error(), "bad.yak:1:4") {
		t.Errorf("error = %v, want the file that does not parse", err)
	}
	if out != messy+"\n" {
		t.Errorf("listed %q, want only %s", out, messy)
	}
	for path, want := range map[string]string{messy: "a: 1\n", tidy: "a: 1\n"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s holds %q, want %q", path, got, want)
		}
	}
}

func TestSchemaImportWritesALibraryTemplatesCanTag(t *testing.T) {
	dir := t.TempDir()
	crd := write(t, dir, "widgets.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: widgets.example.com}
spec:
  group: example.com
  names: {kind: Widget}
  versions:
    - name: v1
      served: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                size: {type: integer, minimum: 1, default: 2}
                owner: {type: string, format: email}
`)
	lib := filepath.Join(dir, "lib", "widgets.yakl")
	if err := os.MkdirAll(filepath.Dir(lib), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "schema", "import", crd, "-o", lib)
	if err != nil {
		t.Fatalf("schema import returned error: %v", err)
	}
	if want := "yak: warning: " + crd + `: WidgetV1.spec.owner: "format" is not imported` + "\n"; out != want {
		t.Errorf("output = %q, want only the warning %q", out, want)
	}
	tmpl := write(t, dir, "app.yak", "import lib.widgets\nweb: !widgets.WidgetV1 {spec: {owner: \"me\"}}\n")
	out, err = run(t, "render", tmpl)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if want := "web:\n  spec:\n    owner: me\n    size: 2\n  apiVersion: example.com/v1\n  kind: Widget\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	bad := write(t, dir, "bad.yak", "import lib.widgets\nweb: !widgets.WidgetV1 {spec: {size: 0}}\n")
	if _, err := run(t, "render", bad); err == nil || !strings.Contains(err.Error(), `field "spec.size" of schema "WidgetV1" is rejected`) {
		t.Errorf("error = %v, want the minimum to refuse a size of 0", err)
	}
}

func TestSchemaImportFormat(t *testing.T) {
	dir := t.TempDir()
	deploy := write(t, dir, "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment\n")

	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "deploy.yaml: cannot tell its format; give --format (crd, jsonschema, openapi)"},
		{[]string{"--format", "crd"}, "found no CustomResourceDefinition in " + deploy},
		{[]string{"--format", "xml"}, `unknown input format "xml"; known formats: crd, jsonschema, openapi`},
	} {
		_, err := run(t, append([]string{"schema", "import", deploy}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("schema import %v: error = %v, want it to contain %q", tc.args, err, tc.want)
		}
	}
}

func TestRenderWritesToFile(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")
	dest := filepath.Join(dir, "out.yaml")

	out, err := run(t, "render", tmpl, "-o", dest)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
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

func TestRenderLeavesOutputFileAloneOnFailure(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "broken.yak", "a: .missing\n")
	dest := write(t, dir, "out.yaml", "previous contents\n")

	if _, err := run(t, "render", tmpl, "-o", dest); err == nil {
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

func TestRenderRequiresOneArgument(t *testing.T) {
	if _, err := run(t, "render"); err == nil {
		t.Error("expected an error when no template is given")
	}
}

func TestRenderErrorMentionsPosition(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "broken.yak", "a: 1\nb: yes\n")

	_, err := run(t, "render", tmpl)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "broken.yak:2:4") {
		t.Errorf("error = %q, want it to point at broken.yak:2:4", err)
	}
}

func TestRenderFormatFlag(t *testing.T) {
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
			out, err := run(t, "render", tmpl, "-f", tc.format)
			if err != nil {
				t.Fatalf("render returned error: %v", err)
			}
			if out != tc.want {
				t.Errorf("stdout = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestWorkdirIsWhereYakRuns checks that "$workdir" is the directory yak was
// invoked from, not the template's.
func TestWorkdirIsWhereYakRuns(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	write(t, dir, "sub/app.yak", "workdir: $workdir\n")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "render", "sub/app.yak")
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if want := "workdir: " + wd + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// TestYakFormatIsCanonical checks that "$yak.format" names the format rather
// than the spelling that chose it.
func TestYakFormatIsCanonical(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "format: $yak.format\n")
	dest := filepath.Join(dir, "out.yml")

	for _, tc := range []struct {
		name string
		args []string
		// toFile marks a case that writes dest rather than stdout.
		toFile bool
		want   string
	}{
		{"default", nil, false, "format: yaml\n"},
		{"flag alias", []string{"-f", "hujson"}, false, "{\n  \"format\": \"jwcc\",\n}\n"},
		{"flag case", []string{"-f", "JSON"}, false, "{\n  \"format\": \"json\"\n}\n"},
		{"output extension", []string{"-o", dest}, true, "format: yaml\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(t, append([]string{"render", tmpl}, tc.args...)...)
			if err != nil {
				t.Fatalf("render returned error: %v", err)
			}
			if tc.toFile {
				got, err := os.ReadFile(dest)
				if err != nil {
					t.Fatal(err)
				}
				out = string(got)
			}
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestRenderDropsNullDocuments(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "let unused = 1\n---\nname: \"web\"\n")

	out, err := run(t, "render", tmpl)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if out != "name: web\n" {
		t.Errorf("stdout = %q, want the null document to be left out", out)
	}
}

func TestRenderKeepsNullDocumentsOnRequest(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "let unused = 1\n---\nname: \"web\"\n")

	out, err := run(t, "render", tmpl, "--keep-null-documents")
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if out != "null\n---\nname: web\n" {
		t.Errorf("stdout = %q, want the null document to be kept", out)
	}
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	_, err := run(t, "render", tmpl, "-f", "xml")
	if err == nil || !strings.Contains(err.Error(), `unknown output format "xml"`) {
		t.Errorf("error = %v, want a complaint about the format name", err)
	}
}

// TestRenderChoosesTheFormat checks the order in which the format is decided:
// -f, then the extension of -o, then the name of the template, then YAML.
func TestRenderChoosesTheFormat(t *testing.T) {
	const (
		toml = "name = \"web\"\n"
		json = "{\n  \"name\": \"web\"\n}\n"
		yaml = "name: web\n"
	)
	tests := []struct {
		name     string
		template string
		output   string
		args     []string
		want     string
	}{
		{"from the output file", "app.yak", "out.toml", nil, toml},
		{"the flag beats the output extension", "app.yak", "out.toml", []string{"-f", "json"}, json},
		{"YAML for an unknown extension", "app.yak", "out.txt", nil, yaml},
		{"from the template name", "app.toml.yak", "", nil, toml},
		{"the output extension beats the template name", "app.toml.yak", "out.json", nil, json},
		{"the template name holds for an unknown extension", "app.toml.yak", "out.txt", nil, toml},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"render", write(t, dir, tc.template, "name: \"web\"\n")}, tc.args...)
			if tc.output != "" {
				args = append(args, "-o", filepath.Join(dir, tc.output))
			}
			got, err := run(t, args...)
			if err != nil {
				t.Fatalf("render returned error: %v", err)
			}
			if tc.output != "" {
				data, err := os.ReadFile(filepath.Join(dir, tc.output))
				if err != nil {
					t.Fatal(err)
				}
				got = string(data)
			}
			if got != tc.want {
				t.Errorf("rendered %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderAutomaticOutput(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")

	out, err := run(t, "render", tmpl, "-O")
	if err != nil {
		t.Fatalf("render returned error: %v", err)
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

func TestRenderAutomaticOutputNamesTheFormat(t *testing.T) {
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
			args := append([]string{"render", tmpl, "-O"}, tc.args...)

			if _, err := run(t, args...); err != nil {
				t.Fatalf("render returned error: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.want)); err != nil {
				t.Errorf("want %s to have been written: %v", tc.want, err)
			}
		})
	}
}

func TestRenderAutomaticOutputRejectsStandardInput(t *testing.T) {
	_, err := run(t, "render", "-", "-O")
	if err == nil || !strings.Contains(err.Error(), "--automatic-output needs a .yak template") {
		t.Errorf("error = %v, want a complaint about the template name", err)
	}
}

func TestRenderRejectsOutputAndAutomaticOutputTogether(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	if _, err := run(t, "render", tmpl, "-O", "-o", filepath.Join(dir, "out.yaml")); err == nil {
		t.Error("expected an error when both -o and -O are given")
	}
}

func TestRenderOutputDirCreatesTheDirectory(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.toml.yak", "name: \"web\"\n")
	outDir := filepath.Join(dir, "build", "nested")

	if _, err := run(t, "render", tmpl, "-O", "--output-dir", outDir); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "app.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "name = \"web\"\n" {
		t.Errorf("file contents = %q, want TOML", got)
	}
}

func TestRenderOutputDirJoinsTheOutputPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	write(t, dir, "app.yak", "name: \"web\"\n")

	if _, err := run(t, "render", "app.yak", "-o", "sub/out.json", "--output-dir", "build"); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "build", "sub", "out.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\n  \"name\": \"web\"\n}\n" {
		t.Errorf("file contents = %q, want JSON", got)
	}
}

func TestRenderOutputDirRejectsAnAbsoluteOutput(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	_, err := run(t, "render", tmpl, "-o", filepath.Join(dir, "out.yaml"), "--output-dir", dir)
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error = %v, want a complaint about the absolute output path", err)
	}
}

func TestRenderOutputDirNeedsAnOutputFile(t *testing.T) {
	dir := t.TempDir()
	tmpl := write(t, dir, "app.yak", "name: \"web\"\n")

	_, err := run(t, "render", tmpl, "--output-dir", dir)
	if err == nil || !strings.Contains(err.Error(), "--output-dir needs") {
		t.Errorf("error = %v, want a complaint about the missing output file", err)
	}
}

// project writes a package holding two templates, a library they import,
// and a hidden template that a directory leaves out.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "yak.pkg", "package:\n  name: web\n")
	write(t, dir, "lib/names.yakl", "app:: \"web\"\n")
	write(t, dir, "app.yak", "import lib.names\nname: names.app\n")
	write(t, dir, "deploy/svc.toml.yak", "import lib.names\nservice: names.app\n")
	write(t, dir, ".hidden/skip.yak", "skipped: true\n")
	return dir
}

func TestRenderDirectoryIntoOutputDir(t *testing.T) {
	dir := project(t)
	out := filepath.Join(t.TempDir(), "build")

	if _, err := run(t, "render", dir, "--output-dir", out); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	for name, want := range map[string]string{
		"app.yaml":        "name: web\n",
		"deploy/svc.toml": "service = \"web\"\n",
	} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	for _, name := range []string{"lib", ".hidden"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s was written, want it left out", name)
		}
	}
}

func TestRenderDirectoryBesideTemplates(t *testing.T) {
	dir := project(t)

	if _, err := run(t, "render", dir, "-O", "-f", "json"); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	for _, name := range []string{"app.json", "deploy/svc.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("want %s to have been written: %v", name, err)
		}
	}
}

func TestRenderDirectoryNeedsAnOutputDirectory(t *testing.T) {
	dir := project(t)

	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "give --output-dir or --automatic-output"},
		{[]string{"-o", filepath.Join(dir, "out.yaml")}, "--output names one file"},
	} {
		_, err := run(t, append([]string{"render", dir}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("render %v: error = %v, want it to contain %q", tc.args, err, tc.want)
		}
	}
}

func TestRenderDirectoryRefusesTwoTemplatesWritingOneFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.yak", "a: 1\n")
	write(t, dir, "app.yaml.yak", "a: 2\n")

	_, err := run(t, "render", dir, "-O")
	if err == nil || !strings.Contains(err.Error(), "would both write") {
		t.Errorf("error = %v, want a complaint about the shared output file", err)
	}
}

func TestRenderDirectoryWritesNothingOnFailure(t *testing.T) {
	dir := project(t)
	write(t, dir, "zz-broken.yak", "a: .missing\n")
	out := filepath.Join(t.TempDir(), "build")

	if _, err := run(t, "render", dir, "--output-dir", out); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("the output directory was written, want nothing written")
	}
}

func TestRenderDirectoryWithoutTemplates(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib.yakl", "a: 1\n")

	_, err := run(t, "render", dir, "-O")
	if err == nil || !strings.Contains(err.Error(), "no .yak templates") {
		t.Errorf("error = %v, want a complaint about the missing templates", err)
	}
}

func TestRenderRefusesALibrary(t *testing.T) {
	dir := project(t)

	_, err := run(t, "render", filepath.Join(dir, "lib", "names.yakl"))
	if err == nil || !strings.Contains(err.Error(), "is a library, which can be imported but not rendered") {
		t.Errorf("error = %v, want a library to be refused", err)
	}
}

func TestRenderDirectorySkipsTheVendorDirectory(t *testing.T) {
	dir := project(t)
	write(t, dir, "vendor/example.com/acme/lib/demo.yak", "demo: true\n")
	write(t, dir, "deploy/vendor/kept.yak", "kept: true\n")
	out := filepath.Join(t.TempDir(), "build")

	if _, err := run(t, "render", dir, "--output-dir", out); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "vendor")); err == nil {
		t.Error("the package's vendor directory was rendered")
	}
	if _, err := os.Stat(filepath.Join(out, "deploy", "vendor", "kept.yaml")); err != nil {
		t.Errorf("a vendor directory outside the package root was skipped: %v", err)
	}
}

// contextPackage writes a package whose context file layers defaults.yaml and,
// for production, prod.yaml beneath the context files the rendering is given.
func contextPackage(t *testing.T) string {
	dir := t.TempDir()
	write(t, dir, "yak.pkg", "package:\n  name: web\n  context: context.yak\n")
	write(t, dir, "defaults.yaml", "name: web\nreplicas: 1\nregion: us\n")
	write(t, dir, "prod.yaml", "replicas: 5\n")
	write(t, dir, "context.yak", `import { std.files, std.yaml }

let load = (f) => yaml.decode(files.read(f))
let env = $$?.env ?? "dev"

<<: load("defaults.yaml") << (if env == "prod" then load("prod.yaml") else {}) << $$
env: env
fullname: "\(env)-\(.name)"
secret:: "hidden"
`)
	return dir
}

func TestRenderEvaluatesThePackageContext(t *testing.T) {
	dir := contextPackage(t)
	tmpl := write(t, dir, "deploy/app.yak", "name: $$.fullname\nreplicas: $$.replicas\nregion: $$.region\nsecret: \"secret\" in $$\n")
	prod := write(t, t.TempDir(), "prod.yaml", "env: prod\nregion: eu\n")

	out, err := run(t, "render", tmpl, "-c", prod)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if want := "name: prod-web\nreplicas: 5\nregion: eu\nsecret: false\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	out, err = run(t, "render", tmpl, "-c", prod, "--skip-package-context")
	if err == nil {
		t.Fatalf("render with --skip-package-context succeeded with %q, want $$.fullname to be missing", out)
	}
}

func TestRenderReadsThePackageContextBackAsData(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "yak.pkg", "package:\n  name: web\n  context: context.yak\n")
	write(t, dir, "context.yak", "s: [\"true\", \"1\", \"null\", \"2024-01-01\", \"a\\nb\"]\nf: 1.0\nn: null\n")
	tmpl := write(t, dir, "app.yak", "import std.json\nv: json.encode($$)\n")

	out, err := run(t, "render", tmpl)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if want := "v: '{\"s\":[\"true\",\"1\",\"null\",\"2024-01-01\",\"a\\nb\"],\"f\":1.0,\"n\":null}'\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRenderChecksThePackageContextSchema(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "yak.pkg", "package:\n  name: web\n  context: context.yak\n")
	write(t, dir, "context.yak", "schema* $$ =\n  env: \"dev\" | \"prod\"\n\n<<: $$\n")
	tmpl := write(t, dir, "app.yak", "env: $$.env\n")
	qa := write(t, dir, "qa.yaml", "env: qa\n")

	if _, err := run(t, "render", tmpl, "-c", qa); err == nil || !strings.Contains(err.Error(), "context.yak") {
		t.Errorf("error = %v, want the context file's schema to refuse env", err)
	}
}

func TestRenderRefusesAPackageContextThatIsNoMapping(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "yak.pkg", "package:\n  name: web\n  context: context.yak\n")
	write(t, dir, "context.yak", "- 1\n")
	tmpl := write(t, dir, "app.yak", "a: 1\n")

	_, err := run(t, "render", tmpl)
	if err == nil || !strings.Contains(err.Error(), "must contain a mapping at the top level, found sequence") {
		t.Errorf("error = %v, want a refusal of the sequence", err)
	}
}

func TestRenderDirectoryLeavesOutThePackageContext(t *testing.T) {
	dir := contextPackage(t)
	write(t, dir, "app.yak", "name: $$.fullname\n")
	out := filepath.Join(t.TempDir(), "build")

	if _, err := run(t, "render", dir, "--output-dir", out); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "context.yaml")); err == nil {
		t.Error("the package context file was rendered as an output")
	}
	got, err := os.ReadFile(filepath.Join(out, "app.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "name: dev-web\n"; string(got) != want {
		t.Errorf("app.yaml = %q, want %q", got, want)
	}
}

func TestRenderThePackageContextItself(t *testing.T) {
	dir := contextPackage(t)

	out, err := run(t, "render", filepath.Join(dir, "context.yak"))
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if want := "name: web\nreplicas: 1\nregion: us\nenv: dev\nfullname: dev-web\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestPackageAddAndRemove(t *testing.T) {
	dir := project(t)
	write(t, dir, "../shared/yak.pkg", "package:\n  name: shared\n")
	write(t, dir, "../base.yak", "region: \"eu\"\n")
	t.Chdir(filepath.Join(dir, "deploy"))

	for _, args := range [][]string{
		{"add", "../../shared"},
		{"pkg", "add", "../../base.yak", "--name", "settings"},
	} {
		if _, err := run(t, args...); err != nil {
			t.Fatalf("yak %s: %v", strings.Join(args, " "), err)
		}
	}
	want := "package:\n  name: web\ndependencies:\n  shared: ../shared\n  settings: ../base.yak\n"
	if got, _ := os.ReadFile(filepath.Join(dir, "yak.pkg")); string(got) != want {
		t.Errorf("yak.pkg = %q, want %q", got, want)
	}
	for _, args := range [][]string{{"remove", "shared"}, {"package", "remove", "settings"}} {
		if _, err := run(t, args...); err != nil {
			t.Fatalf("yak %s: %v", strings.Join(args, " "), err)
		}
	}
	want = "package:\n  name: web\ndependencies: {}\n"
	if got, _ := os.ReadFile(filepath.Join(dir, "yak.pkg")); string(got) != want {
		t.Errorf("yak.pkg = %q, want %q", got, want)
	}
}

func TestPackageCommandsNeedAPackage(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"sync"}, {"add", "x"}, {"pkg", "remove", "x"}} {
		if _, err := run(t, args...); err == nil || !strings.Contains(err.Error(), "no yak.pkg in") {
			t.Errorf("yak %s: error = %v, want a complaint about the missing yak.pkg", strings.Join(args, " "), err)
		}
	}
}

func TestSyncNamesDependenciesOnlyWithUpdate(t *testing.T) {
	t.Chdir(project(t))
	if _, err := run(t, "sync", "lib"); err == nil || !strings.Contains(err.Error(), "only with --update") {
		t.Errorf("error = %v, want a complaint about the name", err)
	}
}

func TestRenderGitPackage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := project(t)
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"add", "-A"},
		{"commit", "--quiet", "-m", "web"},
		{"tag", "v1"},
	} {
		cmd := exec.Command("git", append([]string{"-c", "user.name=yak", "-c", "user.email=yak@example.com",
			"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	ref := "git+file://" + filepath.ToSlash(repo) + "@v1"
	out := filepath.Join(t.TempDir(), "build")

	if _, err := run(t, "render", ref, "-O"); err == nil || !strings.Contains(err.Error(), "give --output-dir") {
		t.Errorf("render -O: error = %v, want --output-dir asked for", err)
	}
	if _, err := run(t, "render", ref, "--output-dir", out); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "deploy", "svc.toml")); string(got) != "service = \"web\"\n" {
		t.Errorf("deploy/svc.toml = %q, want the template rendered", got)
	}
}

func TestInitThenBuild(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "web")
	if _, err := run(t, "init", dir); err != nil {
		t.Fatalf("yak init: %v", err)
	}
	if _, err := run(t, "package", "init", dir); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("yak package init again: error = %v, want a refusal", err)
	}
	t.Chdir(dir)
	if _, err := run(t, "pkg", "build"); err != nil {
		t.Fatalf("yak pkg build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".yak", "oci", "index.json")); err != nil {
		t.Errorf("the build wrote no OCI layout: %v", err)
	}
	out := filepath.Join(t.TempDir(), "build")
	if _, err := run(t, "render", ".", "--output-dir", out); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "app.yaml")); string(got) != "app: web\n" {
		t.Errorf("app.yaml = %q, want the scaffolded template rendered", got)
	}
}

func TestRegistryCommandsTakePlainHTTP(t *testing.T) {
	for _, args := range [][]string{{"render"}, {"add"}, {"sync"}, {"remove"}, {"package", "push"}} {
		cmd, _, err := cli.NewRootCommand().Find(args)
		if err != nil {
			t.Fatal(err)
		}
		if cmd.Flags().Lookup("plain-http") == nil {
			t.Errorf("yak %s has no --plain-http", strings.Join(args, " "))
		}
	}
}
