package engine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/daluz/yak/internal/ctxfile"
	"github.com/daluz/yak/internal/engine"
	"github.com/daluz/yak/internal/parser"
	"github.com/daluz/yak/internal/render"
)

// Example comments group the files of a documentation example in a temporary
// directory. Read the fenced blocks themselves so edits to the docs are tested.
func TestDocExamples(t *testing.T) {
	blockRE := regexp.MustCompile("(?ms)<!-- example: (\\S+) (\\S+) (\\S+) -->\n\n```[^\n]*\n(.*?)^```")
	type block struct {
		role, file, body string
		line             int
	}
	for _, doc := range []string{"README.md", "docs/TOUR.md"} {
		t.Run(doc, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("..", "..", doc))
			if err != nil {
				t.Fatal(err)
			}
			groups := map[string][]block{}
			var names []string
			for _, match := range blockRE.FindAllSubmatchIndex(src, -1) {
				name := string(src[match[2]:match[3]])
				if _, exists := groups[name]; !exists {
					names = append(names, name)
				}
				groups[name] = append(groups[name], block{
					role: string(src[match[4]:match[5]]),
					file: string(src[match[6]:match[7]]),
					body: string(src[match[8]:match[9]]),
					line: bytes.Count(src[:match[8]], []byte("\n")) + 1,
				})
			}
			// Every YAML block in these two docs must declare its role. Other
			// formats are checked when explicitly marked as expected output.
			yamlBlocks := bytes.Count(src, []byte("\n```yaml\n"))
			markedYAML := 0
			for _, match := range blockRE.FindAll(src, -1) {
				if bytes.Contains(match, []byte("\n```yaml\n")) {
					markedYAML++
				}
			}
			if yamlBlocks != markedYAML {
				t.Fatalf("%d YAML blocks, but %d have example comments", yamlBlocks, markedYAML)
			}
			for _, name := range names {
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					var contexts []string
					outputs := map[string][]block{}
					diagnostics := map[string]string{}
					files := map[string]bool{}
					for _, b := range groups[name] {
						path := filepath.Join(dir, b.file)
						if !filepath.IsLocal(b.file) {
							t.Fatalf("%s:%d: example path must be local", doc, b.line)
						}
						if b.role == "output" || strings.HasPrefix(b.role, "output-") {
							outputs[b.file] = append(outputs[b.file], b)
							continue
						}
						if b.role == "diagnostic" {
							diagnostics[b.file] = strings.TrimSpace(strings.TrimPrefix(b.body, "yak: "))
							continue
						}
						if files[b.file] {
							t.Fatalf("%s:%d: duplicate example file %s", doc, b.line, b.file)
						}
						files[b.file] = true
						if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, []byte(b.body), 0o644); err != nil {
							t.Fatal(err)
						}
						switch b.role {
						case "context", "package":
							if _, err := ctxfile.Decode(b.file, []byte(b.body)); err != nil {
								t.Fatalf("%s:%d: %v", doc, b.line, err)
							}
							if b.role == "context" {
								contexts = append(contexts, path)
							}
						case "library", "snippet", "spec":
							if _, err := parser.Parse(b.file, []byte(b.body)); err != nil {
								t.Fatalf("%s:%d: %v", doc, b.line, err)
							}
						case "template", "template-nonstrict", "error":
						default:
							t.Fatalf("%s:%d: unknown example role %q", doc, b.line, b.role)
						}
					}
					for _, b := range groups[name] {
						if b.role != "template" && b.role != "template-nonstrict" && b.role != "error" {
							continue
						}
						wants := outputs[b.file]
						if len(wants) == 0 {
							wants = []block{{role: "output"}}
						}
						for _, want := range wants {
							opts := render.DefaultOptions()
							if format, ok := strings.CutPrefix(want.role, "output-"); ok {
								opts.Format, err = render.ParseFormat(format)
								if err != nil {
									t.Fatal(err)
								}
							}
							var out bytes.Buffer
							err := engine.Template(engine.TemplateRequest{
								Path: filepath.Join(dir, b.file), ContextPaths: contexts,
								Out: &out, Options: opts, NonStrict: b.role == "template-nonstrict",
							})
							if b.role == "error" {
								if err == nil {
									t.Fatalf("%s:%d: example should fail", doc, b.line)
								}
								got := strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), "")
								if expected := diagnostics[b.file]; expected == "" || got != expected {
									t.Errorf("%s:%d: diagnostic\ngot:  %s\nwant: %s", doc, b.line, got, expected)
								}
							} else if err != nil {
								t.Errorf("%s:%d: %v", doc, b.line, err)
							} else if want.file != "" && out.String() != want.body {
								t.Errorf("%s:%d: %s output differs from line %d\n--- got ---\n%s--- want ---\n%s",
									doc, b.line, opts.Format, want.line, out.String(), want.body)
							}
						}
						delete(outputs, b.file)
						delete(diagnostics, b.file)
					}
					if len(outputs) != 0 || len(diagnostics) != 0 {
						t.Fatal("output or diagnostic has no corresponding template")
					}
				})
			}
		})
	}
}
