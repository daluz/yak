# yak

`yak` templatizes YAML. It is similar in spirit to jsonnet, but the surface
language is YAML itself, with the ambiguous parts removed and a small
expression language added.

```yaml
# app.yak
local image = "alpine"
local tag = "3.20"

apiVersion: "apps/v1"
kind: "Deployment"
metadata:
  name: "{$$.app.name}-web"
  labels:
    app: ..name
spec:
  replicas: $$.app?.replicas ?? 1
  selector:
    matchLabels: $.metadata.labels
  containers:
    - image: "{image}:{tag}"
      env:
        - name: "APP_NAME"
          value: $$.app.name
```

```console
$ yak template app.yak -c production.yaml
```

## Install

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

Or build from a checkout:

```console
$ go build ./cmd/yak
```

## Usage

```console
$ yak template FILE.yak [-c context.yaml|context.json]... [-f FORMAT]
                        [-o FILE | -O] [--output-dir DIR]
```

Context files are merged in the order given, with later files taking
precedence, and are available to the template as `$context` or its `$$`
shorthand. They may be YAML or JSON, mixed freely; a `.json` extension selects
the JSON parser. Pass `-` as the file to read a template from standard input.

The rendered output goes to standard output unless `-o` names a file to write.
`-O` names that file after the template instead: the `.yak` extension is
dropped and the one belonging to the format written takes its place, so
`app.toml.yak` yields `app.toml` and `app.yak` yields `app.yaml`.
`--output-dir` places the file in another directory, creating it if it is
missing. [docs/CLI.md](docs/CLI.md) describes the command line in full.

## Output formats

`-f` picks the encoding. Without it the name of the output file decides, and
failing that the name of the template, so `app.toml.yak` writes TOML:

| Format  | Also known as               | Documents | Comments |
| ------- | --------------------------- | --------- | -------- |
| `yaml`  | `yml`                       | many      | yes      |
| `kyaml` |                             | many      | yes      |
| `json`  |                             | one       | no       |
| `jsonc` |                             | one       | `//`     |
| `jwcc`  | `jsoncc`, `hujson`, `json5` | one       | `//`     |
| `jsonl` | `ndjson`                    | many      | no       |
| `toml`  |                             | one       | yes      |

`kyaml` is the Kubernetes dialect from KEP-5295: flow style, quoted string
values, bare keys where they cannot be misread, and trailing commas. `jsonc`
keeps comments, `jwcc` adds trailing commas as well, and `jsonl` writes one
compact document per line.

The single-document formats refuse a template that produced several rather
than picking one. TOML also needs a mapping at the top level and has no null,
so a null value is an error naming the key it came from.

```console
$ yak template app.yak -c production.yaml -f kyaml
$ yak template app.yak -o rendered.json            # the extension chooses JSON
$ yak template app.toml.yak -O                     # writes app.toml
$ yak template app.toml.yak -O --output-dir build  # writes build/app.toml
```

## What makes it different from YAML

- **Strings are always quoted.** An unquoted value is an expression, which is
  what lets `alias: .name` mean a reference instead of the text ".name".
- **Every string interpolates.** `"hello {.name}"`. Double a brace for a
  literal one, and prefix with `r` to turn interpolation off altogether:
  `r"hello {literal}"`.
- **Only `true` and `false` are booleans.** No `yes`, `no`, `on`, or `off`.
- **No anchors and no tags.** References and `local` bindings replace anchors;
  tags return with schemas, and `!` means boolean `not` in the meantime.
- **Relative references.** `.name` is a sibling, `..name` the parent's,
  `...name` the grandparent's, `$.name` the document root's, and `$$.name` the
  context's. `$self`, `$root` and `$context` spell the first, fourth and fifth
  of those out in full.
- **Runtime information is a value.** `$yak` is a mapping holding the
  `version` doing the rendering, the template's `filepath` as given, and the
  `contextpaths` it was handed, which is enough for a provenance note.
- **Hidden fields.** Write `::` instead of `:` to keep a value available to
  references but out of the output, or `:?` to drop an entry only when its
  value is null.
- **Local bindings.** `local name = "web"`, or a `local { ... }` block of
  them. They are named, scoped, and produce no output.
- **Functions.** `local url(host, port = 8080) = "https://{host}:{port}"`,
  called with arguments by position or by name: `url("web", port = 443)`. An
  anonymous one is written `(x) => x * 2`.
- **Built-ins.** `size` measures a sequence, mapping or string, `empty` asks
  whether there is anything in one, and `nullify` turns an empty value into
  `null` for `:?` to drop.
- **Arithmetic, and a `+` that does more.** `+`, `-`, `*`, `/` and `%` work
  on numbers, and `+` also joins two strings or two sequences.
- **Merging.** `<<` merges two mappings at every depth, and the one on the
  right decides: `defaults << {limits: {memory: 512}}`. Written where a key
  belongs it merges into the mapping it is written in, which is YAML's merge
  key without the anchor: a `<<: defaults` among the entries is overridden by
  the ones below it and overrides the ones above.
- **Optional access and defaults.** `?.` and `?[` give `null` where a field or
  an index is missing, and `??` supplies the value to use instead.
- **Conditionals.** `if $$.env == "prod" then 5 else 1`, built on the usual
  `==`, `<`, `&&` and `!` operators. Leave the `else` out and a false
  condition yields `null`, which `:?` drops.
- **Comprehensions.** `[p.name for p in $$.ports if p.tls]` builds a
  sequence, and `{[p.name]: p.number for p in $$.ports}` a mapping.
- **Comments survive.** They are carried into the output, and the ones
  written on a `local` travel to wherever the binding is read. A comment
  starting with `## ` is a note about the template and is left out. Every
  output format that can hold a comment gets them, JSON dialects and TOML
  included.

[docs/SPEC.md](docs/SPEC.md) is the full language description, and
[docs/CLI.md](docs/CLI.md) the tool that renders it.

## Status

The `template` command is implemented, along with `local` bindings,
functions, conditionals, comprehensions and the first built-ins. `local` is
one of three statement keywords; the other two — `import`, which reads
another module into a namespace, and `schema` — are reserved but not
implemented, so using one today fails with a clear message rather than
parsing as something else. They are planned along with the namespaced part of
the standard library and the `build` and `validate` commands; see
[docs/ROADMAP.md](docs/ROADMAP.md).

## Development

```console
$ go test ./...
$ go test ./... -update   # refresh the golden files under testdata/
```

Fixtures live in `testdata/template` (a template, its `*.ctx*.yaml` or
`*.ctx*.json` context files, and the expected `*.want.yaml`) and
`testdata/errors` (a template and the exact diagnostic it should produce).
A template is also rendered in any other format it has a `*.want.FORMAT`
file for, or `*.want.FORMAT.err` where that format has to refuse it.
