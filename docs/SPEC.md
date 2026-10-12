# The yak language

This describes the language as it exists today. Planned features are listed in
[ROADMAP.md](ROADMAP.md); the keywords reserved for them produce an explicit
"not implemented yet" error rather than a parse failure. The command line tool
is documented in [CLI.md](CLI.md).

yak is a restricted dialect of YAML: what YAML does ambiguously is removed or
made explicit, and a small expression language is added on top.

## Files

yak files use the `.yak` extension and hold one or more documents separated by
`---`, optionally terminated by `...`, as in YAML. The name may carry the
extension of an [output format](CLI.md#output-formats) ahead of `.yak`, as
`app.toml.yak` does, to say what the template renders to.

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

### Anchors and tags are gone

`&anchor`, `*alias` and `!!tag` are rejected. References and `local` bindings
replace anchors; tags will return with schemas. A lone `!` is an error that
points to `not`. YAML's `<<` survives as [the merge key](#the-merge-key), and
merges any mapping an expression can produce.

### Keys

A key is one of:

```yaml
plain-key: 1              # any identifier: letters, digits, _, and interior -
"a key with spaces": 2    # any quoted string, interpolation included
["computed-\($$.env)"]: 3  # an expression in brackets
```

Numbers and reserved words must be quoted to be used as keys. Duplicate keys
are an error. `if`, `then`, `else`, `for`, `in`, `and`, `or` and `not` are
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
local defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

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
| `.` | `$self` | The enclosing mapping. |
| `.name` | | The `name` field of the enclosing mapping. |
| `..` / `..name` | | The parent mapping, and its fields. |
| `...` / `...name` | | The grandparent, and so on for each added dot. |
| `$` | `$root` | The root of the current document. |
| `$.name` | | The `name` field of that root. |
| `$$` | `$context` | The merged [context](#context) data. |
| | `$yak` | [Runtime information](#builtin-variables) rather than the document. |

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
`$self.name` is `.name`. Of the dot references only `.` has a long form, and
`$yak` has no sigil. `$super` is reserved and is a "not implemented yet"
error; any other `$name` is an error listing the variables that exist.

`$yak` describes the rendering rather than the document:

| Field | Meaning |
| --- | --- |
| `$yak.version` | The version of yak doing the rendering. |
| `$yak.filepath` | The template's path, exactly as it was given to the tool. |
| `$yak.contextpaths` | The context files, in the order they were given. |
| `$yak.strict` | `true` in [strict mode](STDLIB.md#strict-mode), `false` when it is turned off. |

```yaml
# rendering: yak render deploy/app.yak -c base.yaml -c prod.yaml
a: $yak.filepath            # "deploy/app.yak"
b: $yak.contextpaths[0]     # "base.yaml"
c: "built by yak \($yak.version)"
d: $yak.strict              # true
```

`filepath` is the path as written, not resolved, matching what diagnostics
say; a template read from standard input is `<stdin>`. `contextpaths` is empty,
never absent, when no context file is given. Every document sees the same
`$yak`, and an unstamped build reports its `version` as `dev`. Output that
includes these fields depends on where the template is and which yak rendered
it.

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
| `local` | Names a value or a function. |
| `import` | Reads another module into a namespace. |
| `schema` | Declares types, defaults and validation. |

A statement is not an expression. It stands on its own in its block, renders
nothing, and what it declares is visible to that block and everything nested
inside it. The keywords are reserved: written as a value they are an error,
and as a key they need quotes, as in `"schema": 1`. `import` is described in
[Modules](#modules); `schema` is planned in [ROADMAP.md](ROADMAP.md) and is a
"not implemented yet" error today.

## Local bindings

`local` names a value, and writing the name where a value is expected reads it
back:

```yaml
local name = "web"
local port = 8080

service: name
url: "http://\(name):\(port)"
```

Bindings produce no output, and unlike hidden fields they need not live inside
the mapping that uses them.

Several bindings can share a `local { ... }` block, one per line or separated
by commas:

```yaml
local {
  region = "us-east-1"
  zone = "\(region)a"
}
local { retries = 3, timeout = 30 }
```

A binding's value may be an indented block:

```yaml
local defaults =
  image: "alpine"
  tag: "3.20"

image: "\(defaults.image):\(defaults.tag)"
```

### Scope

A binding covers the block it is written in and everything nested inside it,
and may be used above the line that declares it:

```yaml
url: endpoint          # fine: bindings are resolved lazily too
local endpoint = "https://example.com"
```

Bindings among the entries of a mapping belong to that mapping, so they can
read its fields with `.name`:

```yaml
name: "shop"
local label = "\(.name)-web"
labels:
  app: label
```

An inner binding shadows an outer one of the same name:

```yaml
local tier = "shared"
web:
  local tier = "frontend"
  tier: tier           # "frontend"
api:
  tier: tier           # "shared"
```

Binding a name again in the same block rebinds it from that line down, and the
new value reads the binding it replaces:

```yaml
local size = "small"
dev:
  tier: size           # "small"
local size = "\(size)-ish"
prod:
  tier: size           # "small-ish"
```

A reference reads the nearest binding of its name above it, or the first one
when it is above all of them, so a name bound once can be read anywhere in its
block. A nested block reads at the lines it is written on, which is why `dev`
and `prod` differ. Bindings in one `local { ... }` block count in the order
written.

Every document has its own bindings; nothing carries across a `---`. A
document of nothing but bindings is null and left out of the output. Anywhere
else, a binding must be followed by a value in the same block:

```yaml
local unused = "nothing comes of this"
---
name: "web"
```

Bindings resolve lazily, so they may refer to each other in any order, and one
that is never used is never evaluated. A binding that needs itself is a
circular reference. A rebinding reads the binding it replaces, so a first
`local a = a` is circular and a second one is not.

### Destructuring

A binding may name the items of a sequence by writing a pattern where the name
goes. The outermost brackets may be left out, so these two are the same:

```yaml
local pair = ["web", 8080]
local [name, port] = pair
local name, port = pair

service: "\(name):\(port)"
```

Patterns nest, and `_` stands for an item that binds nothing. Only the
outermost brackets are optional, and not for a single item: `local [a] = x`
takes the one item of `x`, while `local a = x` takes `x` itself.

```yaml
local host, _ = ["registry.example.com", "latest"]
local [[_, tag]] = [["mirror", "1.4"]]
```

In a `local { ... }` block the commas of a pattern do not end the binding, but
brackets read better when bindings share a line:

```yaml
local { bounds = [1, 10], [low, high] = bounds }
```

The value is a single sequence: `local a, b = 1, 2` is an error, written
`local a, b = [1, 2]`.

The sequence must hold exactly as many items as the pattern names, at every
level, and anything other than a sequence where a pattern expects one is an
error. Nothing is evaluated until a name is read; that first read checks the
whole value, so a mismatch is reported the same way whichever name is read.
Items stay lazy unless a nested pattern has to inspect one.

Pattern names are otherwise ordinary bindings: they shadow, rebind and resolve
as a single name does, so `local [a, b] = [b, a]` below earlier bindings of
`a` and `b` swaps them, and comments on the pattern travel to every name it
binds. A name may appear only once in a pattern, and a pattern must bind at
least one name. `_` is not a name: it cannot name a binding or a parameter,
and reading it is an error.

## Functions

A binding whose name is followed by a parameter list is a function. Calling it
evaluates its value with the parameters bound to the arguments:

```yaml
local url(host, port) = "https://\(host):\(port)"

endpoint: url($$.host, 8080)
```

The parentheses of a call are written flush against what they call, as `.` and
`[` are, and the body may be an indented block:

```yaml
local endpoint(name) =
  host: "\(name).example.com"
  port: 443

web: endpoint("web")
```

A parameter may have a default. Arguments are given by position, by name, or
by position and then by name:

```yaml
local url(host, port = 8080, scheme = "https") = "\(scheme)://\(host):\(port)"

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
local span(from, to = from) = "\(from)..\(to)"
```

A parameter may be a [pattern](#destructuring), which destructures its
argument. Commas already separate parameters, so its brackets are required:

```yaml
local address([host, port], scheme = "https") = "\(scheme)://\(host):\(port)"

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
local label(suffix) = "\(.name)-\(suffix)"
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
local apply(fn, to) = fn(to)
local twice(s) = "\(s)\(s)"

v: apply(twice, "ab")   # abab
```

No output format can hold a function, so one that reaches the output is an
error. Calling anything that is not a function is an error naming what was
found.

### Anonymous functions

Where a value belongs, a function is written with its parameters in
parentheses and `=>` before the body:

```yaml
local apply(fn, to) = fn(to)

v: apply((s) => "\(s)\(s)", "ab")   # abab
```

The parameters are those of a binding, defaults and patterns included, so
`([key, value]) => value` takes a pair apart. [What a function
sees](#what-a-function-sees) holds too, so a returned function keeps the
arguments of the call that made it:

```yaml
local adder = (n) => (x) => x + n
v: adder(5)(3)                    # 8
```

The parentheses are required, so `s => "\(s)"` is an error. The body is a
single expression; a function that needs a block body is written as a named
binding. A diagnostic about a call names the function by its binding, or by
where an anonymous one was written.

## Modules

Every `.yak` file is a module, and a directory of them is a package. A dotted
path walks a directory chain and ends at a file, so

```yaml
import mydirectory.mymodule
```

loads `./mydirectory/mymodule.yak` and binds it under the last segment of the
path. Importing a package, such as `mydirectory`, is an error. The binding
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
`local` one:

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
so `$$` reads the same data in both while `$yak.filepath` names the module's
own file. A module is loaded once per document however often it is imported,
and modules may import each other in a cycle as long as no value depends on
itself.

### Resolution

A path resolves from the directory of the importing file and cannot climb out
of it: `mydir.module1` is `./mydir/module1.yak` beside the importing file.

The nearest `yak.mod` in that directory or above maps module paths onto other
locations:

```yaml
modules:
  shared: ../shared            # a package: shared.x.y is ../shared/x/y.yak
  base: vendor/base.yak        # a module: base is vendor/base.yak
  acme: https://example.com/acme
```

A key is a dotted path that covers every import beginning with it, segment by
segment; when several keys do, the longest wins. A target ending in `.yak` is
a module and matches only its own key. Any other local target is a package
directory, in which the rest of the import path is looked up. Local targets
are relative to their `yak.mod`. A path no key covers resolves from the
importing file's directory.

A target written as a URL names a remote package fetched over `https`, `git`
or from an OCI registry. Remote packages are **not implemented yet**: an import
one covers is an error, and the rest of the `yak.mod` still works. A
`yak.lock` pinning remote sources will come with them.

## Standard library

Built-in functions such as `size`, `sort` and `keys` are bare names, and
behave like functions declared in the template: arguments go by position or by
name, and a built-in is a value that can be passed to another function. A
binding shadows a built-in of the same name:

```yaml
local size(v) = "mine"
a: size([1, 2])   # "mine"
```

Everything else lives in modules whose paths begin with `std`. They never
resolve to a file and no `yak.mod` can map them, so a library module is always
found under its own name and a new one cannot take a path a project uses.
Importing one binds a mapping of its functions under the last segment of its
path, which a rename changes as for any import:

```yaml
import str = std.strings
```

Some modules are refused or restricted in [strict mode](STDLIB.md#strict-mode).
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
local defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

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
rendered.

Neither operand changes, and an entry still reads the mapping it was written
in, so an override does not reach back into a sibling that referred to it:

```yaml
local base = {name: "web", label: "\(.name)-1"}

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
mappings or sequences is an error. Ordering works on two numbers or two
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
byName: {[p.name]: p.number for p in $$.ports}
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

The name is an ordinary binding: it shadows an outer one, is visible inside
the element, and comprehensions may nest. A comprehension opens no mapping, so
`.` still names the mapping it is written in.

The name may be a [pattern](#destructuring). Patterns separated by commas are
one pattern without its brackets, so `[k, v]` and `k, v` below mean the same,
and [`items`](STDLIB.md#mappings) turns a mapping into the pairs they walk:

```yaml
local limits = {cpu: 2, memory: 512}
doubled: {[k]: v * 2 for [k, v] in items(limits)}
large: [k for k, v in items(limits) if v > 100]
```

Each item is checked against the pattern as the loop reaches it, whether or
not its names are read. The source and the filter are evaluated as the
comprehension is built; the element stays lazy.

## Context

`$context`, or `$$`, is a mapping of data supplied from outside the template.
It is shared by every document, and the paths it came from are
[`$yak.contextpaths`](#builtin-variables).

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

A `local` renders nothing, so its comments, whether above it, beside it or
inside its value, come out wherever the binding is read back:

```yaml
local {
  # Where everything lands.
  region = "us-east-1"
}

local defaults =
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
local {
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
statement   := local | import
local       := "local" binding | "local" "{" binding+ "}"
binding     := identifier params? "=" value
             | (patterns | pattern ("," pattern)+) "=" value
pattern     := identifier | "_" | patterns
patterns    := "[" pattern ("," pattern)* "]"
import      := "import" module | "import" "{" module+ "}"
module      := (identifier "=")? identifier ("." identifier)*
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
operand     := literal | reference | flowSeq | flowMap | lambda | "(" value ")"
flowSeq     := "[" ((value loop) | (value ("," value)*)? ","?) "]"
flowMap     := "{" ((entry loop) | (entry ("," entry)*)?) "}"
entry       := (key | "<<") sep value
loop        := "for" pattern ("," pattern)* "in" value ("if" value)?
key         := identifier | string | "[" value "]"
literal     := string | int | float | "true" | "false" | "null"
reference   := (dots | builtin | identifier) postfix*
postfix     := "?"? ("." identifier | "[" value "]") | args
args        := "(" (arg ("," arg)* ","?)? ")"
arg         := (identifier "=")? value
dots        := "." | ".." | "..."  ...
builtin     := "$" | "$$" | "$root" | "$self" | "$context" | "$yak"
```
