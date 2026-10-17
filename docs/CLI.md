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
| `yak render PATH` | Render one template, every template in a directory, or a git package. |
| `yak package add SOURCE` | Add a dependency. Also `yak add`. |
| `yak package remove NAME` | Remove a dependency. Also `yak remove`. |
| `yak package sync` | Vendor the remote dependencies and write `yak.lock`. Also `yak sync`. |
| `yak completion SHELL` | Write a shell completion script. |
| `yak help [command]` | Describe a command. |

`--version`, or `-v`, prints the version, the same string a template reads as
`$yak.version`; an unstamped build reports `dev`. `yak pkg` is another name
for `yak package`. `validate`, `fmt`, `package build` and `package push` are
planned; see [ROADMAP.md](ROADMAP.md).

## `yak render`

```console
$ yak render FILE.yak|DIR|GIT-REF [-c context.yaml|context.json]... [-f FORMAT]
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

The one argument is the template to render, a directory of them, or a
[git package](#rendering-a-git-package). `-`
reads a template from standard input; it is then called `<stdin>` by
diagnostics and `$yak.filepath`, and `std.files` reads relative to the
current directory. A `.yakl` file is a [library](SPEC.md#modules), which can
be imported but not rendered, so naming one is an error.

### Rendering a directory

Given a directory, `render` renders every `.yak` template beneath it as it
would render each one alone, with the same context files, format and flags.
Libraries are left out, and so is every file or directory whose name begins
with `.`, and the `vendor/` directory at the root of a package, which holds
other packages' templates. The directory need not be a package.

A directory writes one file per template, so it needs `-O` or
`--output-dir`, and refuses `-o`. Each file is named as `-O` names it. With
`-O` alone it goes beside its template; with `--output-dir` it goes to the
same place in a tree beneath that directory, so `deploy/app.toml.yak` writes
`build/deploy/app.toml`:

```console
$ yak render . --output-dir build -c production.yaml
$ yak render deploy -O
```

Two templates that would write the same file, such as `app.yak` and
`app.yaml.yak`, are an error. Nothing is written unless every template
renders.

### Rendering a git package

Given a [git reference](SPEC.md#remote-packages), `render` clones the
repository into a temporary directory, checks out the ref, vendors the
package's own dependencies as its `yak.lock` pins them, and renders the
package as a directory. It needs `--output-dir`:

```console
$ yak render git+https://github.com/acme/deploy.git//web@v2 --output-dir build
```

## `yak package`

`yak package`, or `yak pkg`, manages the package the current directory is in:
the one whose `yak.pkg` is there or in the nearest directory above it. Its
`add`, `remove` and `sync` subcommands are also top-level commands, so
`yak add` is `yak package add`.

### `yak add`

```console
$ yak add SOURCE [--name NAME]
```

Adds a dependency to `yak.pkg`, keeping the rest of the file, comments
included, as it was. `SOURCE` is a local path, to a package or to a module,
or a [git reference](SPEC.md#remote-packages). The dependency is named after
the package, as its own `yak.pkg` names it, or after the module's file, unless
`--name` gives the import path it goes under. A local path is written
relative to `yak.pkg`. A git package is vendored and pinned in `yak.lock` at
once.

```console
$ yak add git+https://github.com/acme/lib.git@v1.2.0   # named as its yak.pkg names it
$ yak add ../shared --name common
```

### `yak remove`

```console
$ yak remove NAME
```

Removes the dependency `NAME` from `yak.pkg`. A git package leaves `vendor/`
and `yak.lock` too, unless another package still depends on it.

### `yak sync`

```console
$ yak sync [--update [NAME]...]
```

Vendors every git package the package depends on, directly or through
another, into `vendor/` beneath its root, and rewrites `yak.lock`. A package
`yak.lock` pins is checked out at the pinned commit, and one already vendored
with the pinned content is not fetched at all, so `sync` needs no network
when nothing changed. A dependency that `yak.lock` does not pin yet, or whose
ref changed in `yak.pkg`, is fetched at the commit its ref names now. What
`sync` vendors must match the hash `yak.lock` records.

`--update` resolves refs again, for the dependencies it names or, naming
none, for every one: after `yak sync --update lib`, a `lib` that follows a
branch moves to the branch's latest commit.

`sync` runs `git`, so git must be installed, and fetches with the
credentials git already has. Anything in `vendor/` that no dependency needs
is removed, and `vendor/` holds a `.gitignore` so that it is never committed.

### Strict mode

Rendering is strict unless `--strict=false` is given, so the same template and
context always render the same output. Strict mode refuses modules that answer
differently on every rendering, such as `std.random`, and confines
`std.files` to the directory of the file that calls it. Turning it off admits
those modules and lets `std.files` read any path.
[SPEC.md](SPEC.md#strict-mode) has the details.

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
`app.toml.yak` writes TOML. A template reads the chosen format as
[`$yak.format`](SPEC.md#builtin-variables), which is always the name in the
first column below, never one of the others it is known as.

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
