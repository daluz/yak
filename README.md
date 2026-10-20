# yak

`yak` templatizes YAML. It is similar in spirit to jsonnet, but the surface
language is YAML itself, with the ambiguous parts removed and a small
expression language added.

```yaml
# app.yak
let image = "alpine"
let tag = "3.20"

apiVersion: "apps/v1"
kind: "Deployment"
metadata:
  name: "\($$.app.name)-web"
  labels:
    app: ..name
spec:
  replicas: $$.app?.replicas ?? 1
  selector:
    matchLabels: $.metadata.labels
  containers:
    - image: "\(image):\(tag)"
      env:
        - name: "APP_NAME"
          value: $$.app.name
```

```console
$ yak render app.yak -c production.yaml
```

## Install

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

Or build from a checkout with `go build ./cmd/yak`.

## Usage

```console
$ yak render FILE.yak [-c context.yaml|context.json]... [-f FORMAT]
                        [-o FILE | -O] [--output-dir DIR]
```

```console
$ yak render app.yak -c production.yaml -f kyaml
$ yak render app.yak -o rendered.json            # the extension chooses JSON
$ yak render app.toml.yak -O --output-dir build  # writes build/app.toml
```

[docs/CLI.md](docs/CLI.md) describes the command line in full.

## What makes it different from YAML

- **Strings are always quoted.** An unquoted value is an expression, so
  `alias: .name` is a reference rather than the text ".name".
- **Every string interpolates.** `"hello \(.name)"`. Braces are plain text,
  so `"{{ .Values.name }}"` passes through untouched. `\\(` writes a literal
  `\(`, and an `r` prefix turns interpolation off: `r"hello \(literal)"`.
- **Only `true` and `false` are booleans.** No `yes`, `no`, `on`, or `off`.
- **No anchors, and tags are schemas.** References and `let` bindings
  replace anchors, and a tag checks a value against a schema.
- **Relative references.** `.name` is a sibling, `..name` the parent's,
  `...name` the grandparent's, `$.name` the document root's, and `$$.name` the
  context's. `$root` and `$context` (or `$ctx`) are long forms of `$` and
  `$$`.
- **Runtime information.** `$filepath` is the template's path,
  `$contextpaths` the context files and `$workdir` the directory yak ran in;
  `$yak` holds the yak `version`, the output `format` and whether the run is
  `strict`.
- **Hidden fields.** `::` instead of `:` keeps a value available to
  references but out of the output; `:?` drops an entry only when its value is
  null.
- **Let bindings.** `let name = "web"`, or a `let { ... }` block of
  them. They are scoped and produce no output.
- **Functions.** `let url(host, port = 8080) = "https://\(host):\(port)"`,
  called by position or by name: `url("web", port = 443)`. An anonymous one is
  `(x) => x * 2`.
- **Built-ins.** `size`, `empty`, `nempty` (an empty value becomes `null` for
  `:?` to drop), and `render`, which writes a value as the text of an output
  format. Sequences have `sort`, `unique`, `range`, `any` and the like;
  mappings have `keys`, `values`, `items`, `pick` and `omit`.
- **Operators.** `+`, `-`, `*`, `/` and `%` on numbers, with `+` also joining
  strings and sequences; `==`, `<`, `and`, `or`, `not`, and `in` / `not in`
  for sequences, mappings and strings.
- **Merging.** `defaults << {limits: {memory: 512}}` merges two mappings at
  every depth, the right one winning. As a key, `<<: defaults` is YAML's merge
  key without the anchor.
- **Optional access and defaults.** `?.` and `?[` give `null` where a field or
  an index is missing, and `??` supplies a fallback.
- **Conditionals.** `if $$.env == "prod" then 5 else 1`. Without `else`, a
  false condition yields `null`, which `:?` drops.
- **Comprehensions.** `[p.name for p in $$.ports if p.tls]` builds a
  sequence, and `{(p.name): p.number for p in $$.ports}` a mapping.
- **Modules.** `import lib.labels` reads `lib/labels.yak`, and `std` modules
  such as `std.strings`, `std.json`, `std.regex`, `std.files` and
  `std.gotemplate` cover the rest:
  `gotemplate.render(files.read("nginx.conf.tmpl"), {name: $$.name})`.
- **Schemas.** `schema Port = {name: string, protocol: "TCP" | "UDP" = "TCP"}`
  declares a type, and `!Port {name: "http"}` checks a value against it and
  fills in its defaults. `yak schema import` writes the schemas of Kubernetes
  CustomResourceDefinitions as a library.
- **Strict by default.** The same template and context always render the same
  output: `std.random` is refused and `std.files` cannot leave the template's
  directory unless `--strict=false` is given.
- **Comments survive.** They reach every output format that can hold them,
  and the ones on a `let` travel to wherever the binding is read. A comment
  starting with `## ` is a note about the template and is left out.

[docs/TOUR.md](docs/TOUR.md) introduces the language step by step,
[docs/SPEC.md](docs/SPEC.md) defines it, [docs/STDLIB.md](docs/STDLIB.md)
lists the standard library, and [docs/CLI.md](docs/CLI.md) documents the tool.

## Status

The `render`, `package`, `schema check` and `schema import` commands and
everything above are implemented. The `fmt` command is planned; see
[docs/ROADMAP.md](docs/ROADMAP.md).

## Development

```console
$ make test     # go test ./...
$ make update   # refresh the golden files under testdata/
```

Fixtures live in `testdata/template` (a template, its `*.ctx*.yaml` or
`*.ctx*.json` context files, and the expected `*.want.yaml`) and
`testdata/errors` (a template and the exact diagnostic it should produce). A
template is also rendered in any format it has a `*.want.FORMAT` file for, or
`*.want.FORMAT.err` where that format has to refuse it.
