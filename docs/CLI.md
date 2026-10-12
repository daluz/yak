# The yak command line

This describes the tool that renders templates. The language is specified in
[SPEC.md](SPEC.md), and planned commands are listed in
[ROADMAP.md](ROADMAP.md).

## Install

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

Or build from a checkout with `go build ./cmd/yak`.

## Commands

| Command | Purpose |
| --- | --- |
| `yak render FILE` | Render one template. |
| `yak completion SHELL` | Write a shell completion script. |
| `yak help [command]` | Describe a command. |

`--version`, or `-v`, prints the version, the same string a template reads as
`$yak.version`; an unstamped build reports `dev`. `build`, `validate`, `fmt`
and `mod` are planned; see [ROADMAP.md](ROADMAP.md).

## `yak render`

```console
$ yak render FILE.yak [-c context.yaml|context.json]... [-f FORMAT]
                        [-o FILE | -O] [--output-dir DIR]
                        [--keep-null-documents] [--strict=false]
```

| Flag | Meaning |
| --- | --- |
| `-c`, `--context FILE` | Merge a YAML or JSON file into `$context`. Repeatable. |
| `-f`, `--format NAME` | Write this [output format](#output-formats). |
| `-o`, `--output FILE` | Write to this file rather than standard output. |
| `-O`, `--automatic-output` | Write to a file named after the template. |
| `--output-dir DIR` | Put the output file in this directory. |
| `--keep-null-documents` | Write the documents that evaluated to `null`. |
| `--strict=false` | Turn [strict mode](#strict-mode) off. |

`-o` and `-O` cannot be given together.

The one argument is the template to render. `-` reads it from standard input;
it is then called `<stdin>` by diagnostics and `$yak.filepath`, and
`std.files` reads relative to the current directory.

### Strict mode

Rendering is strict unless `--strict=false` is given, so the same template and
context always render the same output. Strict mode refuses modules that answer
differently on every rendering, such as `std.random`, and confines
`std.files` to the directory of the file that calls it. Turning it off admits
those modules and lets `std.files` read any path.
[STDLIB.md](STDLIB.md#strict-mode) has the details.

### Context files

`-c` supplies the data a template reads as `$context`, or `$$` for short.
Repeating it merges the files left to right: mappings merge key by key, and a
sequence or a scalar from a later file replaces what came before.

```console
$ yak render app.yak -c base.yaml -c prod.yaml
```

With `base.yaml` holding `{replicas: 1, env: staging}` and `prod.yaml` holding
`{env: prod}`, the template sees both keys and the later `env`:

```yaml
replicas: $$.replicas       # 1, from base.yaml
env: $$.env                 # "prod", from prod.yaml
region: $$?.region ?? "us-east-1"
```

A template reads the paths as
[`$yak.contextpaths`](SPEC.md#builtin-variables), in the order given.

Context files are plain YAML or JSON, not yak, so they may use anchors and
unquoted strings, and nothing in them interpolates. Each must hold a mapping
at the top level. Several YAML documents, or several JSON values, in one file
merge in order as separate files do.

A `.json` file is read by the JSON parser, which accepts all of JSON,
including escapes such as `\/` that YAML rejects. Any other file is read as
YAML, which accepts most JSON too.

### Where the output goes

The rendered output goes to standard output unless `-o` names a file to write.

`-O` names the output file after the template, replacing its extensions with
that of the format written: `app.toml.yak` yields `app.toml`, `app.yak` yields
`app.yaml`, and `-f json` on `app.toml.yak` yields `app.json`. `-O` with a
template read from standard input is an error.

`--output-dir` puts the output file in a directory, creating it if missing. It
needs `-o` or `-O`, and refuses an absolute output path. With `-O` only the
template's file name is kept, so
`yak render deploy/app.toml.yak -O --output-dir build` writes
`build/app.toml`.

```console
$ yak render app.yak -c production.yaml -f kyaml
$ yak render app.yak -o rendered.json            # the extension chooses JSON
$ yak render app.toml.yak -O                     # writes app.toml
$ yak render app.toml.yak -O --output-dir build  # writes build/app.toml
```

Output is held until every document has rendered, so a failure never leaves a
half-written file.

### Output formats

`-f` selects the format. Without it the output file's extension does, and
failing that the extension the template's name carries ahead of `.yak`, so
`app.toml.yak` writes TOML.

| Format  | Also known as                 | Documents | Comments |
| ------- | ----------------------------- | --------- | -------- |
| `yaml`  | `yml`                         | many      | yes      |
| `kyaml` |                               | many      | yes      |
| `json`  |                               | one       | no       |
| `jsonc` |                               | one       | `//`     |
| `jwcc`  | `jsoncc`, `hujson`, `json5`   | one       | `//`     |
| `jsonl` | `ndjson`                      | many      | no       |
| `toml`  |                               | one       | yes      |

Key order, hidden fields and the all-or-nothing rule are the same in every
format, and comments reach every format that can hold them.

`kyaml` is the Kubernetes dialect of YAML from KEP-5295: flow style
throughout, every string value double-quoted, keys bare unless they could be
misread, trailing commas, and a `---` header on every document. Any YAML
parser reads it. Unlike `kubectl`, yak keeps keys in source order rather than
sorting them.

`jsonc` is JSON with comments, and `jwcc` ("JSON with commas and comments")
adds trailing commas. `json5` is an alias of `jwcc` because everything yak
writes reads as JSON5. `jsonl` writes one compact document per line, which is
how a stream of documents comes out as JSON. The single-document formats are
an error, rather than picking one, when a template produced several.

TOML cannot hold everything yak can say:

- A document must be a mapping, and there can only be one of them.
- There is no null. A null value is an error naming the key it was found
  at; `??` supplies something else, and `:?` drops the key instead.
- Sub-tables must follow the plain keys of their table, so keys come out
  grouped by whether they open a table, in source order within each group.

### Null documents

A document that evaluates to `null`, because it holds only bindings, nothing
at all, or a body written as `null`, is left out unless
`--keep-null-documents` is given. If every document is dropped, the
single-document formats report the empty output as an error.

## Diagnostics

A run writes the whole output or none of it: nothing is written unless every
document evaluates and renders successfully.

A failure is reported on standard error, prefixed with `yak: `, with exit
status 1. A diagnostic with a position names the file, line and column, using
the path as given on the command line.
