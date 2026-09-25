# The yak language

This describes the language as it exists today. Features that are planned but
not yet implemented are listed in [ROADMAP.md](ROADMAP.md); using one of their
keywords produces an explicit "not implemented yet" error rather than a
confusing parse failure.

yak is a restricted dialect of YAML. Everything that YAML does ambiguously has
been either removed or made explicit, and a small expression language has been
added on top.

## Files

| Extension | Meaning |
| --- | --- |
| `.yak` | A template that can be rendered directly. |
| `.libyak` | A library meant to be imported. `yak template` refuses to render one. |

A file holds one or more documents separated by `---`, optionally terminated by
`...`, exactly as in YAML.

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

Any string may contain `${ expression }`:

```yaml
name: "web"
title: "the ${.name} service"
url: "https://${$$.host}:${$$.port}/"
```

To write a literal `${`, double the dollar sign: `"$${not an expression}"`
produces `${not an expression}`. A lone `$` needs no escaping. Inside
double-quoted strings `\$` also works.

To turn interpolation off for a whole string, prefix it with `r`. Raw strings
also disable backslash escapes:

```yaml
pattern: r"\d+ and ${literal}"
script: r|
  echo "${SHELL_VARIABLE}"
```

Because a raw double-quoted string has no escapes, it cannot contain a `"`; use
`r'...'` or a raw block scalar instead.

### Only `true` and `false` are booleans

`yes`, `no`, `on`, `off`, `y`, and `n` are not booleans. Writing one is an
error that tells you to use `true`/`false` or add quotes.

### Anchors and tags are gone

`&anchor`, `*alias`, `!tag`, and `!!tag` are all rejected. Anchors are replaced
by references and `local` bindings. Tags will return with schemas.

### Keys

A key is one of:

```yaml
plain-key: 1              # any identifier: letters, digits, _, and interior -
"a key with spaces": 2    # any quoted string, interpolation included
["computed-${$$.env}"]: 3 # an expression in brackets
```

Numbers and reserved words must be quoted to be used as keys. Duplicate keys
are an error rather than a silent overwrite.

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
    image: "${$.defaults.image}:${$.defaults.tag}"
```

renders as:

```yaml
services:
  web:
    image: alpine:3.20
```

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

## Local bindings

`local` names a value. The name is an ordinary identifier, and writing it
where a value is expected reads the binding back:

```yaml
local name = "web"
local port = 8080

service: name
url: "http://${name}:${port}"
```

Bindings produce no output of their own. They are the replacement for YAML
anchors, and unlike a hidden field they do not have to live inside the mapping
that uses them.

Several bindings can share a `local { ... }` block. Write one per line, or
separate them with commas to fit them on one:

```yaml
local {
  region = "us-east-1"
  zone = "${region}a"
}
local { retries = 3, timeout = 30 }
```

A binding's value may be an indented block, exactly like a mapping value:

```yaml
local defaults =
  image: "alpine"
  tag: "3.20"

image: "${defaults.image}:${defaults.tag}"
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
local label = "${.name}-web"
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

Because they resolve lazily, bindings may refer to each other in any order,
and a binding that ends up needing itself is reported as a circular reference.
A binding that is never used is never evaluated.

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
  at; `??` is how to supply something else.
- Sub-tables have to follow the plain keys of the table they belong to, so
  keys come out grouped by whether they open a table. Within each group the
  source order holds.

## Grammar sketch

```
stream      := document ("---" document)* "..."?
document    := node
node        := local* (blockMapping | blockSequence | value)
blockMapping:= (local | key (":" | "::") value)+  -- aligned on one column
blockSequence := ("-" node)+                      -- aligned on one column
local       := "local" binding | "local" "{" binding+ "}"
binding     := identifier "=" value
value       := operand ("??" operand)*
operand     := literal | reference | flowSeq | flowMap | "(" value ")"
flowSeq     := "[" (value ("," value)*)? ","? "]"
flowMap     := "{" (key (":"|"::") value ("," ...)*)? "}"
key         := identifier | string | "[" value "]"
literal     := string | int | float | "true" | "false" | "null"
reference   := (dots | "$" | "$context" | "$$" | identifier) postfix*
postfix     := "?"? ("." identifier | "[" value "]")
dots        := "." | ".." | "..."  ...
```
