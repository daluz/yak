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
| `yak render PATH` | Render one template, every template in a directory, or a remote package. |
| `yak package init [DIR]` | Make a directory a package. Also `yak init`. |
| `yak package add SOURCE` | Add a dependency. Also `yak add`. |
| `yak package remove NAME` | Remove a dependency. Also `yak remove`. |
| `yak package sync` | Vendor the remote dependencies and write `yak.lock`. Also `yak sync`. |
| `yak package build` | Build the package as an OCI artifact. |
| `yak package push REF` | Push the build to an OCI registry. |
| `yak schema check PATH...` | Check the schemas of templates and libraries without rendering them. |
| `yak schema import FILE...` | Translate Kubernetes CustomResourceDefinitions into a library of schemas. |
| `yak completion SHELL` | Write a shell completion script. |
| `yak help [command]` | Describe a command. |

`--version`, or `-v`, prints the version, the same string a template reads as
`$yak.version`; an unstamped build reports `dev`. `yak pkg` is another name
for `yak package`. `fmt` is planned; see [ROADMAP.md](ROADMAP.md).

## `yak render`

```console
$ yak render FILE.yak|DIR|REMOTE [-c context.yaml|context.json]... [-f FORMAT]
                                   [-o FILE | -O] [--output-dir DIR]
                                   [--keep-null-documents] [--strict=false]
                                   [--skip-schema-checks] [--skip-default-context]
                                   [--plain-http]
```

| Flag | Meaning |
| --- | --- |
| `-c`, `--context FILE` | Merge a YAML or JSON file into `$context`. Repeatable. |
| `--skip-default-context` | Leave out the [default context](#default-context) of the template's package. |
| `-f`, `--format NAME` | Write this [output format](#output-formats). |
| `-o`, `--output FILE` | Write to this file rather than standard output. |
| `-O`, `--automatic-output` | Write to a file named after the template. |
| `--output-dir DIR` | Put the output file in this directory. |
| `--keep-null-documents` | Write the documents that evaluated to `null`. |
| `--strict=false` | Turn [strict mode](#strict-mode) off. |
| `--skip-schema-checks` | Fill in the defaults [schemas](SPEC.md#schemas) declare without checking any value against them. |
| `--plain-http` | Fetch an `oci://` package over HTTP rather than HTTPS. |

`-o` and `-O` cannot be given together.

The one argument is the template to render, a directory of them, or a
[remote package](#rendering-a-remote-package). `-`
reads a template from standard input; it is then called `<stdin>` by
diagnostics and `$filepath`, and `std.files` reads relative to the
current directory. A `.yakl` file is a [library](SPEC.md#modules-and-packages), which can
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

### Rendering a remote package

Given a [git or OCI reference](SPEC.md#remote-packages), `render` fetches
the package into a temporary directory, vendors its own dependencies as its
`yak.lock` pins them, and renders it as a directory. It needs
`--output-dir`:

```console
$ yak render git+https://github.com/acme/deploy.git//web@v2 --output-dir build
$ yak render oci://ghcr.io/acme/web:1.4.0 --output-dir build
```

### Strict mode

Rendering is strict unless `--strict=false` is given, so the same template and
context always render the same output. Strict mode refuses modules that answer
differently on every rendering, such as `std.random`, and confines
`std.files` to the directory of the file that calls it. Turning it off admits
those modules and lets `std.files` read any path.
[SPEC.md](SPEC.md#strict-mode) has the details.

### Context files

`-c` supplies the data a template reads as `$context` or `$ctx`, or `$$` for
short.
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
[`$contextpaths`](SPEC.md#builtin-variables), in the order given.

Context files are plain YAML or JSON, not yak, so they may use anchors and
unquoted strings, and nothing in them interpolates. Each must hold a mapping
at the top level. Several YAML documents, or several JSON values, in one file
merge in order as separate files do.

A `.json` file is read by the JSON parser, which accepts all of JSON,
including escapes such as `\/` that YAML rejects. Any other file is read as
YAML, which accepts most JSON too.

### Default context

A package can list default context files in its `yak.pkg`, as
[`package.context`](SPEC.md#packages), relative to `yak.pkg`:

```yaml
package:
  name: web
  context: [defaults.yaml, regions.json]
```

Every template in the package, rendered alone, as part of a directory or
from a remote package, is rendered with those files merged first, in the
order listed, so each `-c` file overrides them. `$contextpaths` lists them
first too. `--skip-default-context` leaves them out, so that only the `-c`
files are read.

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

## `yak package`

`yak package`, or `yak pkg`, manages the package the current directory is in:
the one whose `yak.pkg` is there or in the nearest directory above it. Its
`init`, `add`, `remove` and `sync` subcommands are also top-level commands,
so `yak add` is `yak package add`.

The commands that reach an OCI registry, `render`, `add`, `remove`, `sync` and
`push`, take `--plain-http` to talk to a registry over HTTP, such as one on
`localhost`, and log in with the credentials that `podman login` or
`docker login` saved. They look for a registry's credentials in these files,
in order, and use the first that has them:

| File | Written by |
| --- | --- |
| `$REGISTRY_AUTH_FILE` | `podman login`, `buildah login` and `skopeo login` when it is set |
| `$XDG_RUNTIME_DIR/containers/auth.json` | `podman login` on Linux |
| `$XDG_CONFIG_HOME/containers/auth.json`, or `~/.config/containers/auth.json` | `podman login` on macOS and Windows |
| `$DOCKER_CONFIG/config.json`, or `~/.docker/config.json` | `docker login` |

A file that does not exist is skipped. A file may name a credential helper,
under `credHelpers` or `credsStore`, as both tools allow. `REGISTRY_AUTH_FILE`
is the way to point yak, and Podman, at an auth file somewhere else:

```console
$ export REGISTRY_AUTH_FILE=$HOME/secrets/registry.json
$ podman login ghcr.io
$ yak package push oci://ghcr.io/acme/web
```

### `yak init`

```console
$ yak init [DIR] [--name NAME]
```

Makes `DIR`, or the current directory, a package. It writes a `yak.pkg` named
after the directory, or `NAME`, at version `0.1.0`; an `app.yak` to start
from, unless there is one; and a `.gitignore` that leaves out `vendor/` and
`.yak/`, adding those lines to one that is already there. It refuses a
directory that already holds a `yak.pkg`, and a directory name that is not
an identifier unless `--name` is given.

### `yak add`

```console
$ yak add SOURCE [--name NAME]
```

Adds a dependency to `yak.pkg`, keeping the rest of the file, comments
included, as it was. `SOURCE` is a local path, to a package or to a module,
or a [git or OCI reference](SPEC.md#remote-packages). The dependency is named after
the package, as its own `yak.pkg` names it, or after the module's file, unless
`--name` gives the import path it goes under. A local path is written
relative to `yak.pkg`. A remote package is vendored and pinned in `yak.lock`
at once.

```console
$ yak add git+https://github.com/acme/lib.git@v1.2.0   # named as its yak.pkg names it
$ yak add ../shared --name common
```

### `yak remove`

```console
$ yak remove NAME
```

Removes the dependency `NAME` from `yak.pkg`. A remote package leaves `vendor/`
and `yak.lock` too, unless another package still depends on it.

### `yak sync`

```console
$ yak sync [--update [NAME]...]
```

Vendors every remote package the package depends on, directly or through
another, into `vendor/` beneath its root, and rewrites `yak.lock`. A package
`yak.lock` pins is fetched at the pinned commit or digest, and one already vendored
with the pinned content is not fetched at all, so `sync` needs no network
when nothing changed. A dependency that `yak.lock` does not pin yet, or whose
ref changed in `yak.pkg`, is fetched at the commit its ref names now. What
`sync` vendors must match the hash `yak.lock` records.

`--update` resolves refs again, for the dependencies it names or, naming
none, for every one: after `yak sync --update lib`, a `lib` that follows a
branch moves to the branch's latest commit.

`sync` runs `git` for a git package, so git must be installed, and fetches
with the credentials git already has. Anything in `vendor/` that no
dependency needs is removed, and `vendor/` holds a `.gitignore` so that it is
never committed, and a `yak.lock` recording what it holds, which rendering
checks against the package's own.

### `yak package build`

```console
$ yak package build [--tag TAG] [-o PATH]
```

Packs the package into an [OCI artifact](SPEC.md#remote-packages) tagged with
`package.version`, or `TAG`, in the OCI image layout `.yak/oci/` beneath the
package root, where `push` finds it. The artifact holds every file of the
package but hidden ones and `vendor/`, `yak.lock` included. `-o` writes the
layout somewhere else instead: a directory, or a tar archive when `PATH` ends
in `.tar`, which tools such as `oras` read.

### `yak package push`

```console
$ yak package push oci://REGISTRY/REPOSITORY[:TAG]
```

Uploads the build in `.yak/oci/` tagged `TAG`, which is `package.version`
when the reference gives none, to that tag of the repository:

```console
$ yak package build
$ yak package push oci://ghcr.io/acme/web    # pushes ghcr.io/acme/web:1.4.0
```

## `yak schema check`

```console
$ yak schema check FILE.yak|FILE.yakl|DIR...
```

Checks the [schemas](SPEC.md#schemas) that each template and library declares,
and the tags it writes, without rendering it or reading a context. Every
schema a type or a tag names has to be in scope, a module's included, and
every `<<` in a mapping type has to merge a mapping type without coming back
to itself. A schema is checked whether or not anything uses it. A directory
checks every `.yak` template and `.yakl` library beneath it, skipping what
`yak render` skips. Values are not checked, since that needs rendering:

```console
$ yak schema check .
yak: lib/k8s.yakl:2:10: no schema or type "strng" is in scope
```

## `yak schema import`

```console
$ yak schema import FILE.yaml|-... [-o FILE] [--format FORMAT]
```

| Flag | Meaning |
| --- | --- |
| `-o`, `--output FILE` | Write the library to this file instead of standard output. |
| `--format FORMAT` | What the files hold. `crd`, Kubernetes CustomResourceDefinitions, is the one format so far. |

Translates the Kubernetes CustomResourceDefinitions in each file into one
`.yakl` library, which a template [imports](SPEC.md#schemas-in-modules) to tag
its objects. `-` reads standard input, a file may hold several documents, and
a `List` of definitions, as `kubectl get crd -o yaml` writes, is read as well.
Only `apiextensions.k8s.io/v1` definitions are read.

Without `--format`, each file is detected: one holding a
CustomResourceDefinition, alone or in a `List`, is `crd`, and a file that is
none of the formats is an error asking for `--format`. Every file of one
import is in the same format, since they make one library.

Every served version becomes a schema named for its kind and version, so a
`Widget` served as `v1` and `v1beta1` gives `WidgetV1` and `WidgetV1beta1`;
two definitions that would name the same schema are an error. A schema's
`apiVersion` and `kind` are the one value each can be, and are filled in when
left out. Its `metadata` takes any mapping, since every object takes the same
metadata and the API server checks it.

```console
$ yak schema import widgets.yaml -o lib/widgets.yakl
yak: warning: widgets.yaml: WidgetV1.spec.owner: "format" is not imported
```

```yaml
# Generated by yak schema import from widgets.yaml.

schema WidgetV1 =
  apiVersion: "example.com/v1" = "example.com/v1"
  kind: "Widget" = "Widget"
  metadata:? mapping
  spec:?
    size: integer if (v) => v >= 1 = 2
    owner:? string
```

```yaml
import lib.widgets

web: !widgets.WidgetV1
  metadata: {name: "web"}
  spec: {owner: "me"}
```

The OpenAPI schema of each version translates as follows:

| OpenAPI | yak |
| --- | --- |
| `type` | `string`, `integer`, `boolean`, `integer \| float` for `number`, a sequence type for `array`, a mapping type for `object`. |
| `properties`, `required` | A field for each property; one that is not required is written `:?`. |
| `default` | The field's default, which also makes it optional. |
| `enum` | A union of the values. |
| `nullable` | `\| null`. |
| `additionalProperties` | A `(string): V` member, or `(string): any` for `true`. |
| `x-kubernetes-preserve-unknown-fields` | `mapping` for an object with no properties, and a `(string): any` member for one with them. |
| `x-kubernetes-int-or-string` | `integer \| string`. |
| `x-kubernetes-embedded-resource` | Optional `apiVersion`, `kind` and `metadata` fields. |
| `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf` | A check on a number. |
| `minLength`, `maxLength`, `pattern` | A check on a string; the library imports `std.regex` for a pattern. |
| `minItems`, `maxItems`, `uniqueItems`, `x-kubernetes-list-type: set` | A check on a sequence. |
| `minProperties`, `maxProperties` | A check on a mapping. |

A check that applies to one type lets the values of the others through, as
OpenAPI does, so `pattern` on an `x-kubernetes-int-or-string` field checks
only strings. An object with neither properties nor
`x-kubernetes-preserve-unknown-fields` takes no keys, as the API server prunes
them all.

Descriptions, titles, examples, `x-kubernetes-list-map-keys` and
`x-kubernetes-map-type` say nothing a value is checked against and are left
out. Any other keyword is left out with a warning on standard error naming
the schema and the field it was written on, such as
`x-kubernetes-validations`, `format`, `anyOf` and `oneOf`, and so is a pattern
that is not RE2, a default yak cannot write, the uniqueness of a `map` list's
keys, and every document that is not a CustomResourceDefinition. A file
holding no definition at all is an error.

## Diagnostics

A run writes the whole output or none of it: nothing is written unless every
document evaluates and renders successfully.

A failure is reported on standard error, prefixed with `yak: `, with exit
status 1. A diagnostic with a position names the file, line and column, using
the path as given on the command line.
