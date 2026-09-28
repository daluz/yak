# The yak command line

This describes the tool that renders templates. The language it renders is
specified in [SPEC.md](SPEC.md), and the commands that are planned but not
built yet are listed in [ROADMAP.md](ROADMAP.md).

## Install

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

Or build from a checkout:

```console
$ go build ./cmd/yak
```

## Commands

| Command | Purpose |
| --- | --- |
| `yak template FILE` | Render one template. |
| `yak completion SHELL` | Write a shell completion script. |
| `yak help [command]` | Describe a command. |

`--version`, or `-v`, prints the version doing the rendering, which is the
same string a template reads as `$yak.version`; a build that was not stamped
calls itself `dev`.

`template` is the only command that renders anything today. `build`,
`validate`, `fmt` and `mod` are planned; see [ROADMAP.md](ROADMAP.md).

## `yak template`

```console
$ yak template FILE.yak [-c context.yaml|context.json]... [-f FORMAT]
                        [-o FILE | -O] [--output-dir DIR]
                        [--keep-null-documents]
```

| Flag | Meaning |
| --- | --- |
| `-c`, `--context FILE` | Merge a YAML or JSON file into `$context`. Repeatable. |
| `-f`, `--format NAME` | Write this [output format](#output-formats). |
| `-o`, `--output FILE` | Write to this file rather than standard output. |
| `-O`, `--automatic-output` | Write to a file named after the template. |
| `--output-dir DIR` | Put the output file in this directory. |
| `--keep-null-documents` | Write the documents that evaluated to `null`. |

`-o` and `-O` name the same thing two different ways and cannot be given
together.

The one argument is the template to render. `-` reads it from standard input
instead of from a file, in which case it is called `<stdin>` by both the
diagnostics and `$yak.filepath`.

### Context files

`-c` supplies the data a template reads as `$context`, or `$$` for short.
Repeating it merges the files left to right: mappings merge key by key, and a
sequence or a scalar from a later file replaces what came before.

```console
$ yak template app.yak -c base.yaml -c prod.yaml
```

With `base.yaml` holding `{replicas: 1, env: staging}` and `prod.yaml` holding
`{env: prod}`, the template sees both keys and the later `env`:

```yaml
replicas: $$.replicas       # 1, from base.yaml
env: $$.env                 # "prod", from prod.yaml
region: $$?.region ?? "us-east-1"
```

The paths are readable as [`$yak.contextpaths`](SPEC.md#builtin-variables), in the order
they were given.

Context files are plain YAML or JSON rather than yak, so they may use anchors
and unquoted strings, and nothing in them interpolates. They must hold a
mapping at the top level. A file that holds several YAML documents, or several
JSON values one after another, is merged in order exactly as separate files
are.

A `.json` name is read by the JSON parser, so all of JSON is accepted,
including escapes such as `\/` that YAML rejects. Every other name is read as
YAML, which accepts most JSON as well.

### Where the output goes

The rendered output goes to standard output unless `-o` names a file to write.

`-O` names the output file after the template: the `.yak` extension is dropped
and the one belonging to the format written takes its place, so `app.toml.yak`
yields `app.toml` and `app.yak` yields `app.yaml`. The format actually written
is what decides, so `-f json` on `app.toml.yak` yields `app.json`. A template
read from standard input leaves nothing to name the file after and is an error.

`--output-dir` places the output file in another directory, creating it if it
is missing. It needs `-o` or `-O` to have a file to place, and it refuses an
absolute output path. With `-O` only the template's own name is kept, so
`yak template deploy/app.toml.yak -O --output-dir build` writes
`build/app.toml`.

```console
$ yak template app.yak -c production.yaml -f kyaml
$ yak template app.yak -o rendered.json            # the extension chooses JSON
$ yak template app.toml.yak -O                     # writes app.toml
$ yak template app.toml.yak -O --output-dir build  # writes build/app.toml
```

The output is held until every document has rendered, so a failure part of the
way through evaluation never leaves a half-written file behind.

### Output formats

`-f` selects the encoding. Without it the extension of the output file selects
one, and failing that the extension the template's own name carries ahead of
`.yak`, so that `app.toml.yak` writes TOML.

| Format  | Also known as                 | Documents | Comments |
| ------- | ----------------------------- | --------- | -------- |
| `yaml`  | `yml`                         | many      | yes      |
| `kyaml` |                               | many      | yes      |
| `json`  |                               | one       | no       |
| `jsonc` |                               | one       | `//`     |
| `jwcc`  | `jsoncc`, `hujson`, `json5`   | one       | `//`     |
| `jsonl` | `ndjson`                      | many      | no       |
| `toml`  |                               | one       | yes      |

Key order, hidden fields and the all-or-nothing rule are the same everywhere.
Comments are carried into every format that has somewhere to put them; the
ones that do not simply leave them out.

`kyaml` is the Kubernetes dialect of YAML from KEP-5295: flow style
throughout, every string value double-quoted, keys left bare unless they
could be read as something else, trailing commas, and a `---` header on every
document. It is a subset of YAML, so any YAML parser reads it. Unlike
`kubectl`, yak writes the keys in the order they were written rather than
sorting them.

`jsonc` adds comments to JSON, and `jwcc` ("JSON with commas and comments")
adds trailing commas on top of them. `json5` names `jwcc` because everything
yak writes reads as JSON5, even though JSON5 as a language allows more than
yak ever emits. `jsonl` writes one compact document per line and is the way
to get a stream of documents out as JSON. The three single-document formats
report an error rather than picking one when a template produced several.

TOML is the one format that cannot hold everything yak can say:

- A document must be a mapping, and there can only be one of them.
- There is no null. A null value is an error naming the key it was found
  at; `??` supplies something else, and `:?` drops the key instead.
- Sub-tables have to follow the plain keys of the table they belong to, so
  keys come out grouped by whether they open a table. Within each group the
  source order holds.

### Null documents

A document that evaluates to `null`, whether it held nothing but bindings,
nothing at all, or a body written as `null`, is left out.
`--keep-null-documents` writes it instead. Dropping every document leaves the
output empty, which the single-document formats report as an error.

## Diagnostics

Nothing is written unless every document evaluates and renders successfully,
so a run either produces the whole output or produces none of it.

A failure is reported on standard error, prefixed with `yak: `, and the exit
status is 1. A diagnostic that has a position in a file names the file, the
line and the column, using the path as it was given on the command line rather
than resolved against the working directory.
