# The yak language

This describes the language as it exists today. The command line tool is
documented in [CLI.md](CLI.md).

yak is a restricted dialect of YAML: what YAML does ambiguously is removed or
made explicit, and a small expression language is added on top.

## Contents

- [Files](#files)
- [Strict mode](#strict-mode)
- [What changed from YAML](#what-changed-from-yaml)
    - [Strings must be quoted](#strings-must-be-quoted)
    - [Every string interpolates](#every-string-interpolates)
    - [Only `true` and `false` are booleans](#only-true-and-false-are-booleans)
    - [Anchors and YAML's tags are gone](#anchors-and-yamls-tags-are-gone)
    - [Keys](#keys)
    - [Hidden fields](#hidden-fields)
    - [Optional fields](#optional-fields)
    - [The merge key](#the-merge-key)
- [References](#references)
    - [Builtin variables](#builtin-variables)
    - [Evaluation order](#evaluation-order)
- [Statements](#statements)
- [Let bindings](#let-bindings)
    - [Scope](#scope)
    - [Destructuring](#destructuring)
- [Functions](#functions)
    - [What a function sees](#what-a-function-sees)
    - [Functions as values](#functions-as-values)
    - [Anonymous functions](#anonymous-functions)
- [Modules and Packages](#modules-and-packages)
    - [Modules](#modules)
    - [Packages](#packages)
    - [Resolution](#resolution)
    - [Remote packages](#remote-packages)
- [Schemas](#schemas)
    - [Declaring a schema](#declaring-a-schema)
    - [Types](#types)
    - [Mapping types](#mapping-types)
    - [Open schemas](#open-schemas)
    - [Tags](#tags)
    - [The document and the context](#the-document-and-the-context)
    - [When values are checked](#when-values-are-checked)
    - [Schemas in modules](#schemas-in-modules)
- [Standard library](#standard-library)
- [Missing values](#missing-values)
- [Operators](#operators)
    - [Arithmetic](#arithmetic)
    - [Merging](#merging)
    - [Comparisons](#comparisons)
    - [Boolean operators](#boolean-operators)
    - [Precedence](#precedence)
- [Conditionals](#conditionals)
- [Comprehensions](#comprehensions)
- [Context](#context)
- [Comments](#comments)
    - [Comments on a binding](#comments-on-a-binding)
    - [Notes about the template](#notes-about-the-template)
- [Output](#output)
- [Grammar sketch](#grammar-sketch)

## Files

yak files use the `.yak` extension and hold one or more documents separated by
`---`, optionally terminated by `...`, as in YAML. The name may carry the
extension of an [output format](CLI.md#output-formats) ahead of `.yak`, as
`app.toml.yak` does, to say what the template renders to.

## Strict mode

Rendering is strict unless the tool is told otherwise
([CLI.md](CLI.md#strict-mode)). In strict mode the same template, context and
files always render the same output, so the result is safe to review as a
diff and to re-render in a GitOps loop.

Strict mode does two things. [Importing](#modules-and-packages) a module whose answers can
change between renderings, such as [`std.random`](STDLIB.md#stdrandom) and
[`std.env`](STDLIB.md#stdenv), is an
error, raised when the import is first read. And
[`std.files`](STDLIB.md#stdfiles) reads only beneath the directory of the file
that calls it. Both errors name strict mode as the reason and say how to turn
it off. Every other module behaves the same in either mode. A template can
check if strict-mode is enabled with [`$yak.strict`](#builtin-variables).

## What changed from YAML

### Strings must be quoted

An unquoted scalar is never a string. It is an expression:

```yaml
a: "web"    # the string "web"
b: 42       # the integer 42
c: .a       # a reference to the sibling field "a"
d: web      # an error: unknown identifier "web"
```

So `name: hello world` is reported as a missing pair of quotes rather than
accepted. All the YAML quoting forms are available:

```yaml
double: "escapes \t and \u00e9 are processed"
single: 'and so they are here'
literal: |
  line breaks are preserved
folded: >
  these lines are
  joined with spaces
```

Single and double quotes mean the same thing, as in Python: both process the
backslash escapes of YAML's double-quoted strings, and both interpolate. They
differ only in which quote has to be escaped:

```yaml
apostrophe: "it's"
quoted: 'say "hi"'
both: 'it\'s "quoted"'
```

YAML's doubled quote is not an escape: `'it''s'` is an error that says to
write `\'`. The escapes are:

| Escape | Produces |
| --- | --- |
| `\n`, `\t`, `\r` | A line feed, a tab, a carriage return. |
| `\0`, `\a`, `\b`, `\v`, `\f`, `\e` | NUL, bell, backspace, vertical tab, form feed, escape. |
| `\"`, `\'`, `\\`, `\/`, `\ ` | The character itself; `\ ` is a space. |
| `\xHH`, `\uHHHH`, `\UHHHHHHHH` | A byte, or a code point, in 2, 4 or 8 hexadecimal digits. |
| `\N`, `\_`, `\L`, `\P` | Next line, non-breaking space, line separator, paragraph separator. |
| `\` at the end of a line | Nothing: the line break and the next line's indentation are dropped. |

Any other backslash sequence, such as `\d`, is an error. Block scalars have no
escapes, and accept the chomping and indentation indicators `|-`, `|+`, `>-`,
`>+` and `|2`.

### Every string interpolates

Any string may contain `\( expression )`:

```yaml
name: "web"
title: "the \(.name) service"
url: "https://\($$.host):\($$.port)/"
```

Braces, `$` and every other character are plain text, so embedded templates
and queries pass through untouched:

```yaml
summary: "{{ $labels.instance }} is down"   # a Prometheus template
query: "up{job=\"api\"}"
script: |
  echo "${SHELL_VARIABLE}"
```

`\\(` writes a literal `\(` in every interpolating form, so
`"\\(not an expression)"` renders `\(not an expression)`. In a quoted string
that is the ordinary `\\` escape followed by `(`. In a block scalar it is the
only sequence a backslash changes: `\\n` there is still two backslashes and an
`n`. A `\(` that is never closed is an error.

A string prefixed with `r` is raw: it neither interpolates nor processes
backslash escapes.

```yaml
pattern: r"^\(\d+\)$"
script: r|
  sed 's/\(v[0-9]*\)/x/'
```

A raw string cannot contain its own quote: write `r'...'` for text with a `"`
in it, `r"..."` for text with a `'`, and a raw block scalar for both.

### Only `true` and `false` are booleans

`yes`, `no`, `on`, `off`, `y` and `n` are errors that say to use
`true`/`false` or add quotes.

### Anchors and YAML's tags are gone

`&anchor`, `*alias` and `!!tag` are rejected. References and `let` bindings
replace anchors. A `!` written against a name is a [tag](#tags), which
checks a value against a schema; any other `!` is an error that points to
`not`. YAML's `<<` survives as [the merge key](#the-merge-key), and
merges any mapping an expression can produce.

### Keys

A key is one of:

```yaml
plain-key: 1              # any identifier: letters, digits, _, and interior -
"a key with spaces": 2    # any quoted string, interpolation included
($$.env + "-only"): 3     # an expression in parentheses
```

Numbers and reserved words must be quoted to be used as keys. An expression in
parentheses must evaluate to a string. Duplicate keys are an error. `if`, `then`, `else`, `for`, `in`, `and`, `or` and `not` are
keywords only where a value is expected, so `for: 3` is an ordinary entry, but
they cannot name a binding or a loop variable.

### Hidden fields

`::` instead of `:` hides an entry from the output while leaving it visible to
references:

```yaml
defaults::
  image: "alpine"
  tag: "3.20"
services:
  web:
    image: "\($.defaults.image):\($.defaults.tag)"
```

renders as:

```yaml
services:
  web:
    image: alpine:3.20
```

### Optional fields

`:?` hides an entry only when its value is `null`, so an entry read from the
context is left out when the context says nothing about it:

```yaml
replicas:? $$?.replicas
selector:? $$?.selector
```

With a context of `{replicas: 3}` that renders as:

```yaml
replicas: 3
```

Only `null` hides the entry; `false`, `0`, `""`, `[]` and `{}` do not. An
entry that is left out takes its comments with it.

### The merge key

`<<` where a key belongs merges a mapping into the one being written, as
YAML's merge key does, without an anchor:

```yaml
let defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

container:
  <<: defaults
  tag: "1.27"
  limits:
    memory: 512
```

renders as:

```yaml
container:
  image: nginx
  tag: "1.27"
  limits:
    cpu: 1
    memory: 512
```

A mapping reads top to bottom: the merge overrides the entries above it, and
the entries below it override the merge, following the [merge](#merging)
rules. `limits` is a mapping on both sides, so `memory` comes from the entry
and `cpu` from the merge.

A key the merge brings in takes the position of the `<<`, and a key already
there keeps its own. A mapping may hold several `<<`, and a later one
overrides an earlier one.

The entry modifiers apply to the merge itself. `<<::` hides everything it
brings in from the output but not from references, and `<<:?` merges nothing
when its value is null:

```yaml
container:
  <<:: defaults            # readable by a reference, never rendered
  <<:? $$?.overrides       # nothing to merge when the context omits it
  cpu: .limits.cpu         # 1, read out of what the merge brought in
```

`<<` needs the keys of the mapping it names to place them, so that mapping,
and any entry of the same mapping it decides, is evaluated where the `<<` is
written rather than when the result is read. A quoted `"<<"` is an ordinary
key.

## References

| Syntax | Long form | Meaning |
| --- | --- | --- |
| `.` | | The enclosing mapping. |
| `.name` | | The `name` field of the enclosing mapping. |
| `..` / `..name` | | The parent mapping, and its fields. |
| `...` / `...name` | | The grandparent, and so on for each added dot. |
| `$` | `$root` | The root of the current document. |
| `$.name` | | The `name` field of that root. |
| `$$` | `$context`, `$ctx` | The merged [context](#context) data. |

Further [builtin variables](#builtin-variables) describe the run rather than
the document.

Sequences do not count as a level, so `..` always names the nearest enclosing
mapping:

```yaml
name: "root"
items:
  - label: ..name   # "root", not the sequence
```

Each document has its own `$`; `$context` is shared by all of them.

Field and index access chain freely, written without spaces:

```yaml
a: $.ports[0]
b: $.ports[-1]          # negative indices count from the end
c: $$["key with space"]
d: $.nested.deeper.name
```

### Builtin variables

A name with a leading `$` is a builtin variable; every other name is a field,
a binding or a parameter. A long form and its sigil are interchangeable, so
`$root.name` is `$.name`. The dot references have no long form, since what
they name depends on where they are written. Any other `$name` is an error
listing the variables that exist.

| Variable | Shorthand | Meaning |
| --- | --- | --- |
| `$root` | `$` | The root of the current document. |
| `$context`, `$ctx` | `$$` | The merged [context](#context) data. |
| `$filepath` | | The path of the file being read, exactly as it was given to the tool. |
| `$contextpaths` | | The context files, in the order they were merged. |
| `$workdir` | | The absolute path of the directory yak was invoked from. |
| `$yak` | | A mapping that describes the rendering, with the fields below. |
| `$yak.version` | | The version of yak doing the rendering. |
| `$yak.strict` | | `true` in [strict mode](#strict-mode), `false` when it is turned off. |
| `$yak.format` | | The [output format](CLI.md#output-formats) being rendered, by its name and never an alias. |

```yaml
# rendering from /work: yak render deploy/app.yak -c base.yaml -c prod.yaml
a: $filepath                # "deploy/app.yak"
b: $contextpaths[0]         # "base.yaml"
c: $workdir                 # "/work"
d: "built by yak \($yak.version)"
e: $yak.strict              # true
f: $yak.format              # "yaml"
```

`$filepath` is the path as written, not resolved, matching what diagnostics
say; a template read from standard input is `<stdin>`. `$contextpaths` is
empty, never absent, when no context file is given. `$workdir` is where yak
was run, not where the template is. Every document sees the same values, and
an unstamped build reports its `version` as `dev`. `format` is the name
however the format was chosen, so `-f hujson` and an output file ending in
`.json5` both give `jwcc`, and `.yml` gives `yaml`. Output that includes
these values depends on where the template is, where yak was run and which
yak rendered it.

### Evaluation order

Values are evaluated lazily, so a reference may point forwards:

```yaml
alias: .name   # fine: evaluated after name is known
name: "web"
```

A value that depends on itself is reported as a circular reference, with the
chain of positions that formed the cycle.

## Statements

Three keywords begin a statement instead of a mapping entry or sequence item:

| Statement | Purpose |
| --- | --- |
| `let` | Names a value or a function. |
| `import` | Reads another module into a namespace. |
| `schema` | Declares types, defaults and validation. |

A statement is not an expression. It stands on its own in its block, renders
nothing, and what it declares is visible to that block and everything nested
inside it. The keywords are reserved: written as a value they are an error,
and as a key they need quotes, as in `"schema": 1`. `import` is described in
[Modules and Packages](#modules-and-packages), and `schema` in
[Schemas](#schemas).

## Let bindings

`let` names a value, and writing the name where a value is expected reads it
back:

```yaml
let name = "web"
let port = 8080

service: name
url: "http://\(name):\(port)"
```

Bindings produce no output, and unlike hidden fields they need not live inside
the mapping that uses them.

Several bindings can share a `let { ... }` block, one per line or separated
by commas:

```yaml
let {
  region = "us-east-1"
  zone = "\(region)a"
}
let { retries = 3, timeout = 30 }
```

A binding's value may be an indented block:

```yaml
let defaults =
  image: "alpine"
  tag: "3.20"

image: "\(defaults.image):\(defaults.tag)"
```

### Scope

A binding covers the block it is written in and everything nested inside it,
and may be used above the line that declares it:

```yaml
url: endpoint          # fine: bindings are resolved lazily too
let endpoint = "https://example.com"
```

Bindings among the entries of a mapping belong to that mapping, so they can
read its fields with `.name`:

```yaml
name: "shop"
let label = "\(.name)-web"
labels:
  app: label
```

An inner binding shadows an outer one of the same name:

```yaml
let tier = "shared"
web:
  let tier = "frontend"
  tier: tier           # "frontend"
api:
  tier: tier           # "shared"
```

Binding a name again in the same block rebinds it from that line down, and the
new value reads the binding it replaces:

```yaml
let size = "small"
dev:
  tier: size           # "small"
let size = "\(size)-ish"
prod:
  tier: size           # "small-ish"
```

A reference reads the nearest binding of its name above it, or the first one
when it is above all of them, so a name bound once can be read anywhere in its
block. A nested block reads at the lines it is written on, which is why `dev`
and `prod` differ. Bindings in one `let { ... }` block count in the order
written.

Every document has its own bindings; nothing carries across a `---`. A
document of nothing but bindings is null and left out of the output. Anywhere
else, a binding must be followed by a value in the same block:

```yaml
let unused = "nothing comes of this"
---
name: "web"
```

Bindings resolve lazily, so they may refer to each other in any order, and one
that is never used is never evaluated. A binding that needs itself is a
circular reference. A rebinding reads the binding it replaces, so a first
`let a = a` is circular and a second one is not.

### Destructuring

A binding may name the items of a sequence by writing a pattern where the name
goes. The outermost brackets may be left out, so these two are the same:

```yaml
let pair = ["web", 8080]
let [name, port] = pair
let name, port = pair

service: "\(name):\(port)"
```

Patterns nest, and `_` stands for an item that binds nothing. Only the
outermost brackets are optional, and not for a single item: `let [a] = x`
takes the one item of `x`, while `let a = x` takes `x` itself.

```yaml
let host, _ = ["registry.example.com", "latest"]
let [[_, tag]] = [["mirror", "1.4"]]
```

In a `let { ... }` block the commas of a pattern do not end the binding, but
brackets read better when bindings share a line:

```yaml
let { bounds = [1, 10], [low, high] = bounds }
```

The value is a single sequence: `let a, b = 1, 2` is an error, written
`let a, b = [1, 2]`.

The sequence must hold exactly as many items as the pattern names, at every
level, and anything other than a sequence where a pattern expects one is an
error. Nothing is evaluated until a name is read; that first read checks the
whole value, so a mismatch is reported the same way whichever name is read.
Items stay lazy unless a nested pattern has to inspect one.

Pattern names are otherwise ordinary bindings: they shadow, rebind and resolve
as a single name does, so `let [a, b] = [b, a]` below earlier bindings of
`a` and `b` swaps them, and comments on the pattern travel to every name it
binds. A name may appear only once in a pattern, and a pattern must bind at
least one name. `_` is not a name: it cannot name a binding or a parameter,
and reading it is an error.

## Functions

A binding whose name is followed by a parameter list is a function. Calling it
evaluates its value with the parameters bound to the arguments:

```yaml
let url(host, port) = "https://\(host):\(port)"

endpoint: url($$.host, 8080)
```

The parentheses of a call are written flush against what they call, as `.` and
`[` are, and the body may be an indented block:

```yaml
let endpoint(name) =
  host: "\(name).example.com"
  port: 443

web: endpoint("web")
```

A parameter may have a default. Arguments are given by position, by name, or
by position and then by name:

```yaml
let url(host, port = 8080, scheme = "https") = "\(scheme)://\(host):\(port)"

a: url("web.example.com")                   # https://web.example.com:8080
b: url("web.example.com", 443)              # https://web.example.com:443
c: url(port = 443, host = "web.example.com")
d: url("web.example.com", scheme = "http")
```

It is an error to follow a named argument with a positional one, to name a
parameter that does not exist or name one twice, to give more arguments than
there are parameters, or to leave out one that has no default.

A default is evaluated at the call, in the scope of the parameters, so it may
name an earlier parameter:

```yaml
let span(from, to = from) = "\(from)..\(to)"
```

A parameter may be a [pattern](#destructuring), which destructures its
argument. Commas already separate parameters, so its brackets are required:

```yaml
let address([host, port], scheme = "https") = "\(scheme)://\(host):\(port)"

a: address(["web.example.com", 443])
```

A pattern parameter is given only by position, and a diagnostic names it by
the pattern as written. Its names count as parameters, so none may repeat
another parameter's name.

### What a function sees

Functions and parameters are bindings, so [Scope](#scope) applies to them, and
a parameter shadows anything of its name the function could otherwise see.

The body is evaluated where the function was written, not where it is called:
`.name` names the mapping that holds the declaration, and names read the
bindings in effect at the declaration.

```yaml
name: "shop"
let label(suffix) = "\(.name)-\(suffix)"
labels:
  app: label("web")
```

Arguments are evaluated lazily in the scope of the call, so an argument no
parameter needs is never evaluated, and neither is a function that is never
called.

### Functions as values

A function is a value: it can be bound to another name, held in a hidden
field, and passed to another function:

```yaml
let apply(fn, to) = fn(to)
let twice(s) = "\(s)\(s)"

v: apply(twice, "ab")   # abab
```

No output format can hold a function, so one that reaches the output is an
error. Calling anything that is not a function is an error naming what was
found.

### Anonymous functions

Where a value belongs, a function is written with its parameters in
parentheses and `=>` before the body:

```yaml
let apply(fn, to) = fn(to)

v: apply((s) => "\(s)\(s)", "ab")   # abab
```

The parameters are those of a binding, defaults and patterns included, so
`([key, value]) => value` takes a pair apart. [What a function
sees](#what-a-function-sees) holds too, so a returned function keeps the
arguments of the call that made it:

```yaml
let adder = (n) => (x) => x + n
v: adder(5)(3)                    # 8
```

The parentheses are required, so `s => "\(s)"` is an error. The body is a
single expression; a function that needs a block body is written as a named
binding. A diagnostic about a call names the function by its binding, or by
where an anonymous one was written.

## Modules and Packages

A template can be split across files. A module is a file that another file
imports to read its values, and a package is a directory of modules with a
`yak.pkg` that names it and lists its dependencies, which may be local
directories, other files, or packages fetched from git or an OCI registry.

### Modules

Every `.yak` and `.yakl` file is a module. A `.yak` file is a template, which
can be rendered as well as imported; a `.yakl` file is a library, which can
only be imported, and is the place for functions and defaults that are never
output of their own. A dotted path walks a directory chain and ends at a
file, so

```yaml
import mydirectory.mymodule
```

loads `mydirectory/mymodule.yak`, or `mydirectory/mymodule.yakl`, and binds it
under the last segment of the path. A path that names both files is an
error, and so is one that names a directory, such as `mydirectory`. The binding
holds the module's value, its one document evaluated as it would be rendered,
and is read with field access. Hidden fields are part of that value, which is
how a module hands out a function or a default without rendering it:

```yaml
import mydirectory.mymodule

image: "\(mymodule.defaults.image):\(mymodule.defaults.tag)"
```

`name = path` chooses the namespace, so two modules with the same file name
can live side by side:

```yaml
import shared = mydirectory.mymodule
```

Several imports can share an `import { ... }` block, as bindings share a
`let` one:

```yaml
import {
  name1 = mydir.module1
  name2 = mydir.module2
}
```

An import is a binding, so [Scope](#scope) applies: it covers its block, an
inner one shadows an outer one, it can be rebound, and a module that is never
read is never loaded. When the last segment of a path cannot name a binding,
write `name = path`.

A module holds exactly one document, evaluated against the importer's context,
so `$$` reads the same data in both while `$filepath` names the module's
own file. A module is loaded once per document however often it is imported,
and modules may import each other in a cycle as long as no value depends on
itself.

### Packages

A package is a directory with a `yak.pkg` file at its root, which names the
package and lists what it depends on:

```yaml
package:
  name: web                    # required; an identifier
  version: 1.4.0               # optional
  description: The web tier    # optional
  context: [defaults.yaml]     # optional; the default context
dependencies:
  shared: ../shared            # a package: shared.x.y is ../shared/x/y.yak
  base: third_party/base.yak   # a module: base is third_party/base.yak
  acme: git+https://github.com/acme/lib.git@v1.2.0
```

A file belongs to the package of the nearest `yak.pkg` in its directory or
above. A file in no package is a module all the same.

`package.context` lists the package's default [context](#context) files,
relative to `yak.pkg` and beneath it. A template the package holds is
rendered with them merged first, in the order listed, before the files the
rendering is given. Only the package of the template being rendered
contributes them: a module it imports reads the importer's context, whatever
package the module is in.

### Resolution

Inside a package, a path resolves from the package root, whichever directory
the importing file is in: `mydir.module1` is `mydir/module1.yak` beneath the
directory holding `yak.pkg`. Outside any package it resolves from the
importing file's directory. Either way a path cannot climb out of where it
starts.

A key of `dependencies` is a dotted path that covers every import beginning
with it, segment by segment; when several keys do, the longest wins. A target
ending in `.yak` or `.yakl` is a module and matches only its own key. Any other
local target must be a package, a directory with a `yak.pkg` of its own, in
which the rest of the import path is looked up; the modules of that package
resolve their own imports from its root. Local targets are relative to their
`yak.pkg`. A path no key covers resolves from the package root.

### Remote packages

A target written as a URL names a remote package. A git package is written

```text
git+SCHEME://HOST/REPOSITORY[//DIR][@REF]
```

where `SCHEME` is `https`, `ssh` or `file`, `DIR` is the directory of the
repository holding the package (its root when left out), and `REF` is a
branch, tag or commit (the default branch when left out):

```yaml
dependencies:
  acme: git+https://github.com/acme/lib.git@v1.2.0
  web: git+ssh://git@gitlab.com/acme/mono.git//packages/web@main
```

A template never fetches anything. `yak sync` (see [CLI.md](CLI.md#yak-sync))
vendors every remote package into `vendor/` beneath the package root, keyed
by its repository's host and path, so `acme` above lands in
`vendor/github.com/acme/lib/`, and an import it covers resolves there. One
that has not been vendored is an error that says to run `yak sync`, and so
is one that `vendor/` holds at another commit or digest than `yak.lock` pins,
or that `yak.lock` pins at another ref than `yak.pkg` asks for: `vendor/`
keeps a `yak.lock` of its own, recording what `yak sync` put there, which
every import is checked against. Files edited by hand inside `vendor/` are not
noticed until the next `yak sync`, which puts them back.

Vendoring is flat. The root package vendors the remote packages its
dependencies depend on too, each once, and a vendored package finds its own
dependencies in that same `vendor/`. A repository is checked out at one ref
only, so two packages that want it at different refs are an error.

`yak.lock`, beside `yak.pkg`, pins every vendored repository to a commit and
a hash of its content, so a checkout of the package with its `yak.lock`
vendors the same files every time:

```yaml
# Written by yak sync. Do not edit it by hand.
packages:
  github.com/acme/lib:
    source: https://github.com/acme/lib.git
    ref: v1.2.0
    commit: 4b825dc642cb6eb9a060e54bf8d69288fbee4904
    hash: sha256:8f43…
```

`yak.lock` is meant to be committed. `vendor/` is not: `yak sync` writes a
`.gitignore` into it, and rebuilds it from `yak.lock`.

An OCI package is written

```text
oci://REGISTRY/REPOSITORY:TAG
oci://REGISTRY/REPOSITORY@sha256:DIGEST
```

and is vendored under the registry's host and the repository, as a git
package is under the repository's host and path; `yak.lock` pins the digest
of its manifest where it pins a git package's commit. It is an OCI artifact
that `yak package build` writes: a manifest of artifact type
`application/vnd.yak.package.v1` with one layer, of media type
`application/vnd.yak.package.layer.v1.tar+gzip`, holding the package's files
but for hidden ones and its `vendor/`. Its `yak.lock` travels with it, so a
package that depends on it vendors the same dependencies it was built with.

## Schemas

A schema describes the values something may hold, and the defaults a mapping
takes for the fields it leaves out. `schema` declares one, and a tag, `!Name`,
checks a value against it:

```yaml
schema Port =
  name: string
  number: integer if (n) => n > 0 and n < 65536
  protocol: "TCP" | "UDP" = "TCP"

ports:
  - !Port {name: "http", number: 8080}
```

renders as:

```yaml
ports:
  - name: http
    number: 8080
    protocol: TCP
```

### Declaring a schema

`schema Name = type` names a type. The type follows the `=` on the same line,
or is a [mapping type](#mapping-types) indented below it, as a binding's
value can be. Several schemas can share a `schema { ... }` block, one per line
or separated by commas, as bindings share a `let` one; a type in the block is
written on its line or in braces:

```yaml
schema Tier = "gold" | "silver"
schema {
  Ports = sequence[Port]
  Labels = {(string): string}
}
```

Two more targets take a schema in place of a name. `schema $ = type` checks
the document, and `schema $$ = type` checks the [context](#context); see [The
document and the context](#the-document-and-the-context). Either may only be
declared at the top of a document, once.

Schemas are looked up apart from bindings: a tag and a type name a schema,
and every other name a binding. A block cannot declare one name as both, nor
declare a schema twice. A schema covers its block and everything nested in
it, as a binding does, and the schemas of a block may name each other in any
order. Its types, defaults and checks read the names in scope where it is
declared. The built-in types and their short names cannot name a schema, nor
can a keyword.

### Types

| Type | Matches |
| --- | --- |
| `any` | Every value. |
| `boolean`, `integer`, `float`, `string`, `sequence`, `mapping`, `function` | A value whose [`type`](STDLIB.md#conversions) is that name. |
| `null`, `true`, `"text"`, `8080`, `0.5` | That value alone, compared as `==` compares it. |
| `sequence[T]` | A sequence whose every item is a `T`. |
| `[A, B, ...]` | A sequence of exactly that many items: an `A`, then a `B`, and so on. |
| A [mapping type](#mapping-types) | A mapping with the fields it declares. |
| `Name`, `module.Name` | What the schema `Name` matches. |
| `A \| B` | What any alternative matches; the first that does decides. |
| `T if f` | A `T` that the function `f` answers `true` for. |

Most built-in types have a short name that means the same:

| Type | Short name |
| --- | --- |
| `boolean` | `bool` |
| `integer` | `int` |
| `string` | `str` |
| `sequence` | `seq` |
| `mapping` | `map` |
| `function` | `func` |

So `seq[int]` is `sequence[integer]`. The `[` is written against the name.
A diagnostic names a type by its full name, as `type` does, whichever name
the schema wrote.

A tuple type, written in brackets, holds two types or more, and may span
lines and end in a comma as a flow sequence does. `[T]` on its own is an
error, so that it is never mistaken for `sequence[T]`:

```yaml
schema Endpoint = [str, int]

primary: !Endpoint ["web.example.com", 443]
```

An integer is not a float, so a field that takes either is written
`integer | float`. A union is written on one line, since a `|` where a value
begins opens a [block scalar](#strings-must-be-quoted). The function of a
check is called with the value and has to answer a boolean.

### Mapping types

A mapping type is written as a block or in braces. Each of its members is a
field, the type of the keys it does not declare, or a merge:

```yaml
schema Service =
  name: string                      # required
  replicas: integer = 1             # left out, it is 1
  host: string = "\(.name).svc"     # a default can read the mapping
  selector:? mapping                # may be left out, or be null
  (string): string                  # every other key holds a string
```

```yaml
schema Deployment =
  <<: Service
  image: string
```

A field with neither a `?` nor a default is required. A default is checked
against the field's type, and is evaluated in the scope of the schema with
`.` naming the mapping being checked, so it can read the other fields. The
fields a mapping takes a default for follow the ones it wrote, in the order
the schema declares them.

A field written `:?` whose value is null counts as left out, so it takes the
default:

```yaml
web: !Service
  name: "web"
  replicas:? $$?.replicas     # 1 when the context has none
```

A key the type does not declare is an error, unless a `(K): V` member covers
it: the key is then checked against `K` and its value against `V`. Entries
written `::` are left alone. `<<: Other` brings in the members of another
mapping type where it is written, and a member below it overrides one of the
same name, as with [the merge key](#the-merge-key).

### Open schemas

`schema*` declares an open schema. Every mapping type it writes accepts the
keys that neither a field nor a `(K): V` member covers, and passes them
through unchecked; a schema it names keeps its own rules. The `*` is written against the keyword, and
`schema* { ... }` makes every schema in its block open. Openness belongs to the
declaration, so an open variant of a closed schema merges it:

```yaml
schema* LooseService =
  <<: Service
```

### Tags

`!Name value` checks `value` against the schema `Name`, and `!module.Name`
against one that an imported module declares. A tag goes wherever a value
does, and one that ends its line applies to the block below it:

```yaml
web: !Service
  name: "web"
api: !Service {name: "api", replicas: 3}
ports: [!Port {name: "http", number: 80}]
```

A tag takes the whole expression after it, so `!Service a << b` checks the
merge of `a` and `b`, and `(!Service a) << b` checks `a` alone.

A tag answers its value with the defaults filled in. A mapping written right
after the tag is that value, so its own entries read the defaults with `.`.
Any other mapping is copied, and the one it was read from keeps no defaults:

```yaml
let base = {name: "web"}
web: !Service base     # base has no "replicas"; web does
```

A tag that names a function is an error saying to call it instead, and a
name that is no schema is an error that points to `not` in case a negation
was meant.

### The document and the context

`schema $ = type` checks the document as it is evaluated, so `$.name` reads
the defaults it fills in. `schema $$ = type` checks the context before
anything reads it, and the whole of it rather than as it is read, since the
context is data rather than expressions. Every reader of `$$` sees the
defaults it fills in, the modules the template imports included. A context
file carries no positions, so an error about one points at the schema.

```yaml
schema* $$ =
  env: "dev" | "prod"
  replicas: integer = 1
```

Only the template being rendered may declare `schema $$`; a module that does
is an error, since what it filled in would change what the template reads.

### When values are checked

Checking is lazy, as evaluation is. The shape of a tagged mapping is checked
when it is first read: the fields it needs, the keys it may hold, and the
defaults it takes, for which its `:?` entries are evaluated. The length of
a sequence that a tuple type checks is part of its shape. Each field and
each item is checked when it is read. Rendering reads every field, so what
reaches the output is always checked, while a binding that nothing reads is
never checked. Two tagged mappings may therefore read each other's fields.

A union picks the first alternative that the value's shape matches, even if
one of its fields fails later.

`yak render --skip-schema-checks` lets every value through, so a schema only
fills in defaults; a union still picks its alternative as above, and a value
that fits none is left as it is. [CLI.md](CLI.md#yak-schema-check) describes
the commands that check schemas without rendering and import them.

### Schemas in modules

The schemas declared at the top of a module are the ones a file importing it
can name, as `module.Name` with the name the import binds. Naming one loads
the module. A module that declares no such schema is an error, and so is a
name that is not an import.

```yaml
import k8s = acme.k8s

web: !k8s.Deployment {name: "web", image: "nginx"}
```

## Standard library

Built-in functions such as `size`, `sort` and `keys` are bare names, and
behave like functions declared in the template: arguments go by position or by
name, and a built-in is a value that can be passed to another function. A
binding shadows a built-in of the same name:

```yaml
let size(v) = "mine"
a: size([1, 2])   # "mine"
```

Everything else lives in modules whose paths begin with `std`. They never
resolve to a file and no `yak.pkg` can map them, so a library module is always
found under its own name and a new one cannot take a path a project uses.
Importing one binds a mapping of its functions under the last segment of its
path, which a rename changes as for any import:

```yaml
import str = std.strings
```

Some modules are refused or restricted in [strict mode](#strict-mode).
[STDLIB.md](STDLIB.md) documents every built-in and every module.

## Missing values

`?.` and `?[` read a field or an index that may not be there, and produce
`null` instead of failing:

```yaml
a: $$?.replicas      # null when the context has no "replicas"
b: $$.tags?[3]       # null when the sequence is shorter than that
c: $$?.tls?.enabled  # null at either step
```

The `?` forgives a missing field and a null on its left, but not a type error:
reading a field of a number, or indexing a sequence with a string, still
fails. Each `?` covers one step, so a chain that may break anywhere needs one
at every step.

`??` supplies a value for when the left side is `null`. `false`, `0` and `""`
do not trigger it, and the right side is evaluated only when needed:

```yaml
replicas: $$?.replicas ?? 1
region: $$?.region ?? "us-east-1"
```

`??` never hides an error: `$$.replicas ?? 1` still fails when the context has
no `replicas`, so an optional field is written `$$?.replicas ?? 1`.

## Operators

### Arithmetic

| Syntax | Meaning |
| --- | --- |
| `x + y` / `x - y` | Sum and difference. |
| `x * y` / `x / y` | Product and quotient. |
| `x % y` | Remainder, taking the sign of `x`. |
| `-x` | Negation. |

Two integers give an integer, and a float on either side gives a float. `/`
always gives a float, so `7 / 2` is `3.5`. Dividing or taking the remainder by
zero is an error, as is an operand that is not a number.

`+` also concatenates two strings or two sequences:

```yaml
name: $$.app + "-web"     # "shop-web"
ports: [80] + [443]       # [80, 443]
```

`+` does not join mappings, which is what `<<` is for, and its error says so.
Adding two values of different kinds, or of a kind `+` does not join such as
booleans, is an error naming both.

### Merging

`x << y` merges two mappings into a new one, and `y` decides. A key held by
both takes its value from `y` and keeps its position in `x`:

```yaml
let defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

container: defaults << {limits: {memory: 512}, port: 8080}
```

renders as:

```yaml
container:
  image: nginx
  limits:
    cpu: 1
    memory: 512
  port: 8080
```

A key whose value is a mapping on both sides is merged at every depth, as
`limits` is; any other value in `y` replaces the one in `x`. Both operands
must be mappings, so `[80] << [443]` is an error naming both kinds. An entry
keeps its modifiers, so a `::` on the right hides an entry the left would have
rendered. The built-in [`merge`](STDLIB.md#mappings) makes the same merge
shallow, or appends sequences held on both sides.

Neither operand changes, and an entry still reads the mapping it was written
in, so an override does not reach back into a sibling that referred to it:

```yaml
let base = {name: "web", label: "\(.name)-1"}

v: (base << {name: "api"}).label   # "web-1", not "api-1"
```

Telling a mapping from anything else needs its value, so a key held by both
sides is evaluated where the `<<` is written rather than where the result is
read. This is the one way `<<` is stricter than other operators; the output is
the same either way.

Written where a key belongs, `<<` is [the merge key](#the-merge-key).

### Comparisons

| Syntax | Meaning |
| --- | --- |
| `x == y` / `x != y` | Equality. |
| `x < y` / `x <= y` | Ordering. |
| `x > y` / `x >= y` | Ordering the other way. |

Equality compares two scalars. Values of different types are never equal,
except that integers and floats compare as numbers, so `1 == 1.0`. Comparing
mappings, sequences or functions is an error. Ordering works on two numbers or two
strings; anything else is an error naming both types.

`x in y` asks whether `y` holds `x`, and `x not in y` means
`not (x in y)`:

| `y` is | `x in y` is true when |
| --- | --- |
| A sequence | One of its items `==` `x`. |
| A mapping | `x` is a string naming one of its keys, hidden ones included. |
| A string | `x` is a string that appears somewhere in `y`. |

For a sequence, `x` must be a scalar, and items that are collections are
skipped. Any other `y` is an error naming its type. Other questions about
strings are in [`std.strings`](STDLIB.md#stdstrings).

```yaml
web: "web" in $$.roles               # a sequence of strings
tls: "tls" in $$.listener            # a key of a mapping
secure: "https://" in $$.url         # a substring
legacy: $$.region not in ["eu", "us"]
```

### Boolean operators

`and`, `or` and `not` take only booleans. There is no truthiness, so `if 1` is
an error. `and` and `or` evaluate their right side only when the left does not
decide the answer. `&&`, `||` and `!` are errors naming the word to write.

### Precedence

Operators bind tighter the further down this list they appear, and every
binary level is left associative:

```
??
or
and
not
==  !=
<  <=  >  >=  in  not in
+  <<  -
*  /  %
-
```

`not` sits below the comparisons, so `not a == b` is `not (a == b)` and
`not x in xs` is `x not in xs`. A `not` operand of a comparison or of
arithmetic needs parentheses: `a == (not b)`. The `-` at the bottom is
negation, written flush against its operand.

`-` may appear inside an identifier, so a subtraction needs whitespace on both
sides:

| Written | Read as |
| --- | --- |
| `a-b` | One name. |
| `a - b` | `a` minus `b`. |
| `a -b` | An error, naming the rule. |

A flush `-` negates, which is what makes `-x` and `1 - -x` work. Where a block
sequence item may begin, `-` is the sequence indicator, and a negative number
is a single literal:

```yaml
a:
  - -1        # the item -1
  - - 1       # a nested sequence
  - (-x)      # the item -x, since "- -x" would nest
```

## Conditionals

`if c then x else y` chooses between two values. The condition must be a
boolean, and the branch not taken is never evaluated:

```yaml
replicas: if $$.env == "prod" then 5 else 1
port: if $$?.port != null then $$.port else 8080
```

Without `else`, a false condition yields `null`, so with `:?` an entry appears
only under some condition:

```yaml
debug:? if $$.env != "prod" then true
```

An `else` may hold another conditional:

```yaml
tier: if .replicas > 3 then "gold" else if .replicas > 1 then "silver" else "bronze"
```

A conditional binds looser than every operator, so as an operand it needs
parentheses: `not (if c then a else b)`. It may span several lines, since
`then` and `else` continue it:

```yaml
banner:
  if $$.env == "prod"
  then "serving live traffic"
  else "not for production use"
```

## Comprehensions

A flow collection whose single element is followed by `for name in source`
evaluates the element once per item of a sequence, with `name` bound to the
item:

```yaml
names: ["port-\(p.name)" for p in $$.ports]
byName: {(p.name): p.number for p in $$.ports}
```

With a context of `{ports: [{name: http, number: 80}]}` that renders as:

```yaml
names:
  - port-http
byName:
  http: 80
```

In a mapping comprehension, two items that produce the same key are a
duplicate key. Any key form works, interpolated strings included.

An `if` clause after the source keeps only the items it accepts:

```yaml
public: [p.name for p in $$.ports if p.number < 1024]
```

Several `for` clauses may follow one another, each with its own optional
`if`. A later clause is walked once per item the clauses before it keep, and
can read their names, so the element is evaluated once per combination:

```yaml
pairs: ["\(p.name)-\(r)" for p in $$.ports if p.number < 1024 for r in ["eu", "us"]]
```

renders as `["http-eu", "http-us"]` with the context above.

The name is an ordinary binding: it shadows an outer one, is visible inside
the element and in every later clause, and comprehensions may nest. A comprehension opens no mapping, so
`.` still names the mapping it is written in.

The name may be a [pattern](#destructuring). Patterns separated by commas are
one pattern without its brackets, so `[k, v]` and `k, v` below mean the same,
and [`items`](STDLIB.md#mappings) turns a mapping into the pairs they walk:

```yaml
let limits = {cpu: 2, memory: 512}
doubled: {(k): v * 2 for [k, v] in items(limits)}
large: [k for k, v in items(limits) if v > 100]
```

Each item is checked against the pattern as the loop reaches it, whether or
not its names are read. The source and the filter are evaluated as the
comprehension is built; the element stays lazy.

## Context

`$context`, `$ctx` or `$$` is a mapping of data supplied from outside the
template. It is shared by every document, and the paths it came from are
[`$contextpaths`](#builtin-variables), the default context of the template's
[package](#packages) first.

Context data is plain YAML or JSON, not yak: a `\(` in a context string is a
`\(`, and an unquoted scalar is the string YAML reads it as. Several sources
merge into one mapping, a later one winning; [CLI.md](CLI.md#context-files)
describes how. Since what a context carries varies between runs, a template
usually reads it with the [missing value](#missing-values) operators:

```yaml
replicas: $$?.replicas ?? 1
selector:? $$?.selector
```

## Comments

A comment runs from `#` to the end of the line, as in YAML, and is carried
into the output. One above an entry stays above it, and one after a value
stays beside it:

```yaml
# The public face of the service.
service:
  name: "web"    # and its DNS label
```

renders as:

```yaml
# The public face of the service.
service:
  name: web # and its DNS label
```

Comments with nothing after them come out at the end of the document. Comments
in context files are not carried over.

### Comments on a binding

A `let` renders nothing, so its comments, whether above it, beside it or
inside its value, come out wherever the binding is read back:

```yaml
let {
  # Where everything lands.
  region = "us-east-1"
}

let defaults =
  # The image every service starts from.
  image: "alpine"

region: region
container: defaults
```

renders as:

```yaml
# Where everything lands.
region: us-east-1
container:
  # The image every service starts from.
  image: alpine
```

A binding is read back when its name is the whole value of an entry or a
sequence item. Named inside a larger expression, such as an interpolation, it
brings no comments.

A comment written where the binding is read comes above whatever the binding
brings, and the binding's beside comment is used only where that place has
none of its own. A binding read in several places brings its comments to each
of them, and one read by a hidden entry brings them nowhere.

### Notes about the template

A comment that starts with `## ` is never written to the output. It addresses
whoever edits the template next, and on a binding it describes the binding
rather than its value:

```yaml
let {
  ## Describes the binding, not what it renders.
  region = "us-east-1"
}
## Revisit when the cluster moves.
region: region
```

renders as:

```yaml
region: us-east-1
```

A space must follow the marker, so `##note` and `### banner` are ordinary
comments, while a bare `##` is the marker with nothing written on it.

## Output

A document renders in the order it was written, with its comments. Hidden
fields are dropped, and so are `:?` entries whose value is null. A document
that evaluates to `null`, whether it holds only bindings, nothing at all, or a
body written as `null`, renders nothing. A file of several documents renders
each in turn.

[CLI.md](CLI.md#output-formats) lists the output formats and how one is
chosen. Every format preserves key order, drops hidden fields, writes nothing
unless every document renders successfully, and carries comments wherever it
has a place for them. A value the format cannot hold is an error, not an
approximation. No format can hold a function; TOML, which has no null, refuses
the most.

## Grammar sketch

```
stream      := document ("---" document)* "..."?
document    := node | statement+
node        := statement* (blockMapping | blockSequence | value)
blockMapping:= (statement | key sep value)+       -- aligned on one column
blockSequence := ("-" node)+                      -- aligned on one column
sep         := ":" | "::" | ":?"
statement   := let | import | schema
let         := "let" binding | "let" "{" binding+ "}"
binding     := identifier params? "=" value
             | (patterns | pattern ("," pattern)+) "=" value
pattern     := identifier | "_" | patterns
patterns    := "[" pattern ("," pattern)* "]"
import      := "import" module | "import" "{" module+ "}"
module      := (identifier "=")? identifier ("." identifier)*
schema      := "schema" "*"? (target "=" type | "{" (target "=" type)+ "}")
target      := identifier | "$" | "$$"
type        := typeAtom ("|" typeAtom)* ("if" value)?
typeAtom    := schemaName | literal | ("sequence" | "seq") "[" type "]" | tuple | typeMapping
tuple       := "[" type ("," type)+ ","? "]"
schemaName  := identifier ("." identifier)?
typeMapping := "{" (member ("," member)*)? "}" | member+  -- aligned on one column
member      := (identifier | string) (":" | ":?") type ("=" value)?
             | "(" type ")" ":" type | "<<" ":" type
tag         := "!" schemaName value
params      := "(" (param ("," param)* ","?)? ")"
param       := (identifier | patterns) ("=" value)?
lambda      := params "=>" value
value       := conditional | coalesce
conditional := "if" coalesce "then" value ("else" value)?
coalesce    := or
or          := and ("or" and)*
and         := not ("and" not)*
not         := "not" not | binary
binary      := unary (op unary)*                  -- see the precedence list
unary       := "-"* operand
operand     := literal | reference | flowSeq | flowMap | lambda | tag | "(" value ")"
flowSeq     := "[" ((value loop) | (value ("," value)*)? ","?) "]"
flowMap     := "{" ((entry loop) | (entry ("," entry)*)?) "}"
entry       := (key | "<<") sep value
loop        := ("for" pattern ("," pattern)* "in" value ("if" value)?)+
key         := identifier | string | "(" value ")"
literal     := string | int | float | "true" | "false" | "null"
reference   := (dots | builtin | identifier) postfix*
postfix     := "?"? ("." identifier | "[" value "]") | args
args        := "(" (arg ("," arg)* ","?)? ")"
arg         := (identifier "=")? value
dots        := "." | ".." | "..."  ...
builtin     := "$" | "$$" | "$root" | "$context" | "$ctx" | "$yak"
             | "$filepath" | "$contextpaths" | "$workdir"
```
