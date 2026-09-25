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
  name: "${$$.app.name}-web"
  labels:
    app: ..name
spec:
  replicas: $$.app?.replicas ?? 1
  selector:
    matchLabels: $.metadata.labels
  containers:
    - image: "${image}:${tag}"
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
$ yak template FILE.yak [-c context.yaml|context.json]... [-o output.yaml]
```

Context files are merged in the order given, with later files taking
precedence, and are available to the template as `$context` or its `$$`
shorthand. They may be YAML or JSON, mixed freely; a `.json` extension selects
the JSON parser. Pass `-` as the file to read a template from standard input.

## What makes it different from YAML

- **Strings are always quoted.** An unquoted value is an expression, which is
  what lets `alias: .name` mean a reference instead of the text ".name".
- **Every string interpolates.** `"hello ${.name}"`. Prefix with `r` to turn
  that off: `r"hello ${literal}"`.
- **Only `true` and `false` are booleans.** No `yes`, `no`, `on`, or `off`.
- **No anchors and no tags.** References and `local` bindings replace anchors;
  tags return with schemas.
- **Relative references.** `.name` is a sibling, `..name` the parent's,
  `...name` the grandparent's, `$.name` the document root's, and `$$.name` the
  context's.
- **Hidden fields.** Write `::` instead of `:` to keep a value available to
  references but out of the output.
- **Local bindings.** `local name = "web"`, or a `local { ... }` block of
  them. They are named, scoped, and produce no output.
- **Optional access and defaults.** `?.` and `?[` give `null` where a field or
  an index is missing, and `??` supplies the value to use instead.
- **Comments survive.** They are carried into the output, except the ones
  written inside a `local` and the ones starting with `#local`, which are
  notes about the template.

[docs/SPEC.md](docs/SPEC.md) is the full language description.

## Status

The `template` command is implemented, along with `local` bindings. Functions,
a standard library, `import`, `schema`, and the `build` and `validate`
commands are planned; see [docs/ROADMAP.md](docs/ROADMAP.md). Their keywords
are already reserved, so using one today fails with a clear message rather
than parsing as something else.

## Development

```console
$ go test ./...
$ go test ./... -update   # refresh the golden files under testdata/
```

Fixtures live in `testdata/template` (a template, its `*.ctx*.yaml` or
`*.ctx*.json` context files, and the expected `*.want.yaml`) and
`testdata/errors` (a template and the exact diagnostic it should produce).
