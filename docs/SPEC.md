# The yak language

This describes the language as it exists today, together with the `import`
statement, which is specified here but not implemented yet. Everything else
that is planned is listed in [ROADMAP.md](ROADMAP.md); using one of the
keywords held for it produces an explicit "not implemented yet" error rather
than a confusing parse failure.

yak is a restricted dialect of YAML. Everything that YAML does ambiguously has
been either removed or made explicit, and a small expression language has been
added on top.

## Files

yak files use the `.yak` extension. A file holds one or more documents
separated by `---`, optionally terminated by `...`, exactly as in YAML.

## What changed from YAML

### Strings must be quoted

An unquoted scalar is never a string. It is an expression, so these are all
different things:

```yaml
a: "web"    # the string "web"
b: 42       # the integer 42
c: .a       # a reference to the sibling field "a"
d: web      # an error: unknown identifier "web"
```

This is the rule that makes references work without inventing a new sigil for
them, and it is why `yak` can tell you that `name: hello world` is a missing
pair of quotes rather than silently accepting it.

All the YAML quoting forms are available:

```yaml
double: "escapes \t and \u00e9 are processed"
single: 'only '' is an escape'
literal: |
  line breaks are preserved
folded: >
  these lines are
  joined with spaces
```

Block scalars accept the usual chomping and indentation indicators: `|-`, `|+`,
`>-`, `>+`, and `|2`.

### Every string interpolates

Any string may contain `{ expression }`:

```yaml
name: "web"
title: "the {.name} service"
url: "https://{$$.host}:{$$.port}/"
```

Double a brace to write a literal one: `{{` produces `{` and `}}` produces
`}`, so `"{{not an expression}}"` renders `{not an expression}`. A closing
brace standing on its own needs no escaping; an opening one that is never
closed is an error rather than literal text.

Doubling is the only escape there is, which is what makes it work identically
in every interpolating form, including single-quoted strings and block
scalars, where backslash escapes do not exist.

To turn interpolation off for a whole string, prefix it with `r`. Raw strings
also disable backslash escapes:

```yaml
pattern: r"\d+ and {literal}"
script: r|
  echo "${SHELL_VARIABLE}"
```

A raw string is usually the better answer for text that is dense with braces,
such as a Go template or a query with a label selector in it.

Because a raw double-quoted string has no escapes, it cannot contain a `"`; use
`r'...'` or a raw block scalar instead.

### Only `true` and `false` are booleans

`yes`, `no`, `on`, `off`, `y`, and `n` are not booleans. Writing one is an
error that tells you to use `true`/`false` or add quotes.

### Anchors and tags are gone

`&anchor`, `*alias`, and `!!tag` are all rejected. Anchors are replaced by
references and `local` bindings. Tags will return with schemas; `!` on its
own is the boolean `not` operator, so only the `!!` form still names a tag.

### Keys

A key is one of:

```yaml
plain-key: 1              # any identifier: letters, digits, _, and interior -
"a key with spaces": 2    # any quoted string, interpolation included
["computed-{$$.env}"]: 3  # an expression in brackets
```

Numbers and reserved words must be quoted to be used as keys. Duplicate keys
are an error rather than a silent overwrite.

`if`, `then`, `else`, `for` and `in` mean something only where a value is
expected, so they stay usable as bare keys: `for: 3` is an ordinary entry.
They cannot name a binding or a loop variable, because reading such a name
back would be read as the keyword instead.

### Hidden fields

Writing `::` instead of `:` hides an entry from the output while leaving it
visible to references. This is how you keep shared defaults next to the data
that uses them:

```yaml
defaults::
  image: "alpine"
  tag: "3.20"
services:
  web:
    image: "{$.defaults.image}:{$.defaults.tag}"
```

renders as:

```yaml
services:
  web:
    image: alpine:3.20
```

### Optional fields

Writing `:?` hides an entry only when its value turns out to be `null`, which
is how an entry that comes from the context is left out when the context says
nothing about it:

```yaml
replicas:? $$?.replicas
selector:? $$?.selector
```

With a context of `{replicas: 3}` that renders as:

```yaml
replicas: 3
```

Only `null` hides the entry; `false`, `0`, `""`, `[]` and `{}` are values like
any other. An entry that is left out takes its comments with it.

## References

| Syntax | Meaning |
| --- | --- |
| `.` | The enclosing mapping. |
| `.name` | The `name` field of the enclosing mapping. |
| `..` / `..name` | The parent mapping, and its fields. |
| `...` / `...name` | The grandparent, and so on for each added dot. |
| `$` / `$.name` | The root of the current document. |
| `$context` / `$$` | The merged context data. |

Sequences do not count as a level, so `..` always names the nearest enclosing
mapping:

```yaml
name: "root"
items:
  - label: ..name   # "root", not the sequence
```

Each document in a file has its own `$`. `$context` is shared by all of them.

Field and index access chain freely, and must be written without spaces:

```yaml
a: $.ports[0]
b: $.ports[-1]          # negative indices count from the end
c: $$["key with space"]
d: $.nested.deeper.name
```

### Evaluation order

Values are evaluated lazily, so a reference may point forwards:

```yaml
alias: .name   # fine: evaluated after name is known
name: "web"
```

A value that ends up depending on itself is reported as a circular reference,
with the chain of positions that formed the cycle.

## Statements

A line in a block is usually a mapping entry or a sequence item. Three
keywords begin a statement instead:

| Statement | Purpose |
| --- | --- |
| `local` | Names a value or a function. |
| `import` | Reads another module into a namespace. |
| `schema` | Declares types, defaults and validation. |

A statement is never an expression. It stands on its own in the block it is
written in, it renders nothing of its own, and what it declares is visible to
that block and to everything nested inside it.

All three keywords are reserved, so one written where a value belongs is an
error rather than a reference, and using one as a key takes quotes:
`"schema": 1`. Only `local` is implemented. `import` is described in
[Modules](#modules) and `schema` in [ROADMAP.md](ROADMAP.md); writing either
one today is a "not implemented yet" error.

## Local bindings

`local` names a value. The name is an ordinary identifier, and writing it
where a value is expected reads the binding back:

```yaml
local name = "web"
local port = 8080

service: name
url: "http://{name}:{port}"
```

Bindings produce no output of their own. They are the replacement for YAML
anchors, and unlike a hidden field they do not have to live inside the mapping
that uses them.

Several bindings can share a `local { ... }` block. Write one per line, or
separate them with commas to fit them on one:

```yaml
local {
  region = "us-east-1"
  zone = "{region}a"
}
local { retries = 3, timeout = 30 }
```

A binding's value may be an indented block, exactly like a mapping value:

```yaml
local defaults =
  image: "alpine"
  tag: "3.20"

image: "{defaults.image}:{defaults.tag}"
```

### Scope

A binding covers the block it is written in, including everything nested
inside it, and it may be used before the line that declares it:

```yaml
url: endpoint          # fine: bindings are resolved lazily too
local endpoint = "https://example.com"
```

Bindings written among the entries of a mapping belong to that whole mapping,
so they can read its fields with `.name`:

```yaml
name: "shop"
local label = "{.name}-web"
labels:
  app: label
```

A binding in an inner block shadows an outer one of the same name. Two
bindings of the same name in one block are an error:

```yaml
local tier = "shared"
web:
  local tier = "frontend"
  tier: tier           # "frontend"
api:
  tier: tier           # "shared"
```

Every document has its own bindings; nothing carries across a `---`.

A document may hold nothing but bindings, in which case it is null and is
left out of the output. Anywhere else a binding still has to be followed by a
value in the same block:

```yaml
local unused = "nothing comes of this"
---
name: "web"
```

Because they resolve lazily, bindings may refer to each other in any order,
and a binding that ends up needing itself is reported as a circular reference.
A binding that is never used is never evaluated.

## Functions

A binding whose name is followed by a parameter list is a function. Calling it
evaluates its value with the parameters bound to the arguments:

```yaml
local url(host, port) = "https://{host}:{port}"

endpoint: url($$.host, 8080)
```

The parentheses of a call are written flush against what they call, exactly as
`.` and `[` are, and the body may be an indented block like any other binding
value:

```yaml
local endpoint(name) =
  host: "{name}.example.com"
  port: 443

web: endpoint("web")
```

A parameter may declare the value to use when a call leaves it out. Arguments
are given by position, by name, or by position and then by name:

```yaml
local url(host, port = 8080, scheme = "https") = "{scheme}://{host}:{port}"

a: url("web.example.com")                   # https://web.example.com:8080
b: url("web.example.com", 443)              # https://web.example.com:443
c: url(port = 443, host = "web.example.com")
d: url("web.example.com", scheme = "http")
```

A named argument may not be followed by a positional one, since its position
would say nothing. Naming a parameter that does not exist, naming one twice,
giving more arguments than there are parameters, and leaving out one that has
no default are all errors.

A default is evaluated at the call, in the same scope as the parameters, so it
may name a parameter declared before it:

```yaml
local span(from, to = from) = "{from}..{to}"
```

### What a function sees

A function is a binding, so everything bindings do applies: it covers the
block it is written in, it may be used before the line that declares it, and
one in an inner block shadows an outer one of the same name. A parameter is a
binding too, and shadows anything of its name that the function could
otherwise see.

The body is evaluated where the function was written, not where it is called,
so `.name` inside one names the mapping that holds the declaration:

```yaml
name: "shop"
local label(suffix) = "{.name}-{suffix}"
labels:
  app: label("web")
```

Arguments are evaluated in the scope of the call instead, and lazily, so an
argument that no parameter needs is never evaluated, and neither is a function
that is never called.

### Functions as values

A function is a value like any other. It can be bound to another name, held in
a hidden field, and passed to another function:

```yaml
local apply(fn, to) = fn(to)
local twice(s) = "{s}{s}"

v: apply(twice, "ab")   # abab
```

No output format can hold a function, so one that reaches the output is an
error rather than something rendered. Calling anything that is not a function
is an error naming what was found instead.

## Standard library

Some functions are built in. They are written without qualification, and they
behave in every other way like a function declared in the template: arguments
go by position or by name, and a built-in is a value that can be passed to
another function.

| Call | Answer |
| --- | --- |
| `size(value)` | How much is in a sequence, a mapping, or a string. |
| `empty(value)` | Whether the value is `null`, `{}`, `[]` or `""`. |
| `nullify(value)` | `null` when `empty(value)`, and the value itself otherwise. |

`size` counts items, entries, or characters:

```yaml
ports: [80, 443]
count: size(.ports)       # 2
label: size("café")       # 4, characters rather than bytes
```

Hidden entries count, because `size` describes the value rather than what the
renderer will write out. A value with nothing to measure — a number, a
boolean, `null`, or a function — is an error naming the type that was found.

`empty` and `nullify` accept anything. Only `null` and the three empty
collections are empty; `0`, `false` and `"abc"` are values like any other,
exactly as they are for `??` and `:?`. Pairing `nullify` with `:?` is how an
entry disappears when the context supplies nothing useful for it:

```yaml
labels:? nullify($$?.labels)
```

With a context of `{labels: {}}` that entry is left out, while `{labels: {app:
web}}` renders it.

Bindings are found before built-ins, so a `local` named after one shadows it
and a template that already uses the name keeps working:

```yaml
local size(v) = "mine"
a: size([1, 2])   # "mine"
```

Built-ins are bare names, which is why there are few of them and why they are
about values in general rather than any one kind of value. The library modules
to come — string, encoding and formatting helpers — will be namespaced
instead, so that growing the library cannot take a bare name out from under a
template.

## Modules

**Not implemented yet.** This is how `import` will be used once it lands.

Every `.yak` file is a module. A module is named by a dotted path that walks a
directory chain and ends at the file, so

```yaml
import mydirectory.mymodule
```

loads `./mydirectory/mymodule.yak` and binds it under the last segment of the
path. The name is an ordinary binding of the module's definitions, read back
with field access like any other mapping:

```yaml
import mydirectory.mymodule

image: "{mymodule.defaults.image}:{mymodule.defaults.tag}"
```

Writing `name = path` chooses the namespace instead of taking it from the
path, which is how two modules of the same file name live side by side:

```yaml
import shared = mydirectory.mymodule
```

Several imports can share an `import { ... }` block, exactly as bindings share
a `local` one. Each names its namespace, one per line or separated by commas:

```yaml
import {
  name1 = mydir.module1
  name2 = mydir.module2
}
```

### Resolution

A path resolves relative to the file that imports it, so `mydir.module1` is
`./mydir/module1.yak` beside the importing file.

A `yak.mod` and `yak.lock` pair changes that. `yak.mod` remaps a module — or
the parent module that contains it — onto somewhere else: another local
directory, or a remote location fetched over `https` or `git`. `yak.lock`
pins what `yak.mod` names so the same paths resolve to the same sources on
every machine. A path with no entry covering it keeps resolving relative to
the importing file.

## Missing values

`?.` and `?[` read a field or an index that may not be there, and produce
`null` instead of failing:

```yaml
a: $$?.replicas      # null when the context has no "replicas"
b: $$.tags?[3]       # null when the sequence is shorter than that
c: $$?.tls?.enabled  # null at either step
```

The `?` forgives a missing field and a null on the left of the access. It does
not forgive a misunderstanding: reading a field of a number is still an error,
and so is indexing a sequence with a string. Each `?` covers one step only, so
a chain that may break anywhere needs one at every step.

`??` supplies a value to use when the left side is `null`:

```yaml
replicas: $$?.replicas ?? 1
region: $$?.region ?? "us-east-1"
```

Only `null` triggers the fallback; `false`, `0`, and `""` are values like any
other. The right hand side is left unevaluated when it is not needed.

`??` never hides an error, which is why the two operators are usually written
together. `$$.replicas ?? 1` still fails when the context has no `replicas`
field, because the failure happens before `??` is reached; `$$?.replicas ?? 1`
is the way to say that the field is optional.

## Operators

Arithmetic works on numbers:

| Syntax | Meaning |
| --- | --- |
| `x + y` / `x - y` | Sum and difference. |
| `x * y` / `x / y` | Product and quotient. |
| `x % y` | Remainder, taking the sign of `x`. |
| `-x` | Negation. |

Two integers answer an integer, and a float on either side answers a float.
`/` is the exception: it always answers a float, so `7 / 2` is `3.5` rather
than a silently truncated `3`. Dividing or taking the remainder by zero is an
error, as is any operand that is not a number.

`+` also joins two values of the same kind. Strings and sequences
concatenate:

```yaml
name: $$.app + "-web"     # "shop-web"
ports: [80] + [443]       # [80, 443]
```

Two mappings merge, and the one on the right decides. A key held by both
takes its value from the right and keeps the position it has on the left, so
an override leaves the shape of the original recognisable:

```yaml
local defaults = {image: "nginx", tag: "1.25"}

container: defaults + {tag: "1.27", port: 8080}
```

That renders as:

```yaml
container:
  image: nginx
  tag: "1.27"
  port: 8080
```

The merge is one level deep. A key whose value is a mapping on both sides is
replaced outright rather than merged, which keeps what `+` did readable from
the two mappings alone. An entry also carries over whatever the mapping it
came from said about it, so a `::` on the right hides an entry the left would
have rendered.

Merging builds a third mapping and leaves both operands as they were. An
entry that came through a merge still reads the mapping it was written in, so
overriding a key does not reach back into a sibling that referred to it:

```yaml
local base = {name: "web", label: "{.name}-1"}

v: (base + {name: "api"}).label   # "web-1", not "api-1"
```

Adding two values of different kinds is an error naming both, and so is
adding two of a kind that `+` does not join, such as two booleans.

Comparisons answer a boolean:

| Syntax | Meaning |
| --- | --- |
| `x == y` / `x != y` | Equality. |
| `x < y` / `x <= y` | Ordering. |
| `x > y` / `x >= y` | Ordering the other way. |

Equality compares two scalars. Values of different types are never equal,
except that an integer and a float compare as the numbers they are, so
`1 == 1.0`. Mappings and sequences have no useful answer and are an error
rather than a silent `false`. Ordering is defined for two numbers or two
strings; anything else is an error naming both types.

`&&`, `||` and `!` combine booleans, and only booleans: yak has no
truthiness, so `if 1` is an error rather than a guess. `&&` and `||`
short-circuit, leaving the right hand side unevaluated whenever the left one
already decides the answer.

Operators bind tighter the further down this list they appear, and every
level is left associative:

```
??
||
&&
==  !=
<  <=  >  >=
+  -
*  /  %
!  -
```

Because `-` may appear inside an identifier and may also negate what follows
it, a subtraction is the one written with whitespace on both sides. The three
spacings each mean something different, and the odd one out is an error
rather than a guess:

| Written | Read as |
| --- | --- |
| `a-b` | One name. |
| `a - b` | `a` minus `b`. |
| `a -b` | An error, naming the rule. |

A `-` written flush against its operand negates it, which is what makes `-x`
and `1 - -x` read as they do. Where a block sequence item may begin, though,
a `-` is the sequence indicator it has always been, and a negative number is
a single literal:

```yaml
a:
  - -1        # the item -1
  - - 1       # a nested sequence
  - (-x)      # the item -x, since "- -x" would nest
```

## Conditionals

`if c then x else y` chooses between two values. The condition must be a
boolean, and the branch that is not taken is never evaluated:

```yaml
replicas: if $$.env == "prod" then 5 else 1
port: if $$?.port != null then $$.port else 8080
```

`else` may be left out, in which case a false condition yields `null`. Paired
with `:?` that is how an entry appears only under some condition:

```yaml
debug:? if $$.env != "prod" then true
```

An `else` may hold another conditional, which is how a chain is written:

```yaml
tier: if .replicas > 3 then "gold" else if .replicas > 1 then "silver" else "bronze"
```

A conditional binds looser than every operator, so one used as an operand has
to say how it groups: `!(if c then a else b)`. It may be spread over several
lines, since `then` and `else` continue the expression they belong to:

```yaml
banner:
  if $$.env == "prod"
  then "serving live traffic"
  else "not for production use"
```

## Comprehensions

A flow collection whose single element is followed by `for name in source`
builds itself by walking a sequence, evaluating the element once per item
with `name` bound to it:

```yaml
names: ["port-{p.name}" for p in $$.ports]
byName: {[p.name]: p.number for p in $$.ports}
```

With a context of `{ports: [{name: http, number: 80}]}` that renders as:

```yaml
names:
  - port-http
byName:
  http: 80
```

The key of a mapping comprehension has to differ from item to item, because
two items that produce the same key are a duplicate rather than an
overwrite. Any key form works, so an interpolated string is as good as the
bracketed one.

An `if` clause after the source keeps only the items it accepts:

```yaml
public: [p.name for p in $$.ports if p.number < 1024]
```

The name is an ordinary binding, so it shadows an outer one, it is visible to
anything nested inside the element, and a comprehension may appear inside
another one. A comprehension opens no mapping of its own, so `.` still names
the mapping the comprehension is written in.

The source is walked as the comprehension is built, and so is the filter,
which means both are resolved eagerly; the element itself stays as lazy as
any other value.

## Context files

`yak template -c a.yaml -c b.yaml` merges context files left to right. Mappings
merge key by key; sequences and scalars from a later file replace what came
before. The result is available as `$context`, or `$$` for short.

Context files are plain YAML or JSON, not yak, so they may use anchors and
unquoted strings. They must have a mapping at the top level.

A `.json` file is read by the JSON parser, so all of JSON is accepted,
including escapes such as `\/` that YAML rejects. Every other file is read as
YAML, which accepts most JSON as well.

## Comments

A comment runs from `#` to the end of the line, as in YAML, and it is carried
into the rendered output. One written above an entry stays above it, and one
written after a value stays beside it:

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

A comment belongs to the next thing that is rendered, so one written above a
`local` comes out above whatever follows the binding. Comments with nothing
after them come out at the end of the document.

Two kinds of comment are left out of the output:

- Anything written **inside a `local`**: in a `local { ... }` block, or in
  the value of a binding. A binding renders nothing, so there is nowhere to
  put them, and its value may be used in several places at once.
- Any comment that starts with **`#local`**, wherever it is written. This is
  how to address the next person to edit the template rather than whoever
  reads the output. A word boundary is required after the marker, so
  `#localhost` is an ordinary comment.

```yaml
local {
  # Dropped: this describes the bindings, not the output.
  region = "us-east-1"
}
#local Revisit when the cluster moves.
region: region
```

renders as:

```yaml
region: us-east-1
```

Because a comment travels with the entry it was written on, a value that is
used in several places brings its comments along to each of them. Comments in
context files are not carried over at all.

## Output

`yak template` writes YAML by default: source key order is preserved,
comments are kept, hidden fields are dropped, and documents are separated by
`---`. Nothing is written unless every document evaluates successfully.

A document that evaluates to `null`, whether it held nothing but bindings,
nothing at all, or a body written as `null`, is left out.
`--keep-null-documents` writes it instead. Dropping every document leaves the
output empty, which the single-document formats report as an error.

`--format` selects another encoding, and naming an output file with a known
extension selects the matching one:

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
import      := "import" module | "import" "{" module+ "}"   -- not implemented
module      := (identifier "=")? identifier ("." identifier)*
params      := "(" (param ("," param)* ","?)? ")"
param       := identifier ("=" value)?
value       := conditional | coalesce
conditional := "if" coalesce "then" value ("else" value)?
coalesce    := binary ("??" binary)*
binary      := unary (op unary)*                  -- see the precedence list
unary       := ("!" | "-")* operand
operand     := literal | reference | flowSeq | flowMap | "(" value ")"
flowSeq     := "[" ((value loop) | (value ("," value)*)? ","?) "]"
flowMap     := "{" ((entry loop) | (entry ("," entry)*)?) "}"
entry       := key sep value
loop        := "for" identifier "in" value ("if" value)?
key         := identifier | string | "[" value "]"
literal     := string | int | float | "true" | "false" | "null"
reference   := (dots | "$" | "$context" | "$$" | identifier) postfix*
postfix     := "?"? ("." identifier | "[" value "]") | args
args        := "(" (arg ("," arg)* ","?)? ")"
arg         := (identifier "=")? value
dots        := "." | ".." | "..."  ...
```
