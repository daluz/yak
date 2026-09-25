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
by references and, later, by `local` bindings. Tags will return with schemas.

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

## Context files

`yak template -c a.yaml -c b.yaml` merges context files left to right. Mappings
merge key by key; sequences and scalars from a later file replace what came
before. The result is available as `$context`, or `$$` for short.

Context files are plain YAML or JSON, not yak, so they may use anchors and
unquoted strings. They must have a mapping at the top level.

A `.json` file is read by the JSON parser, so all of JSON is accepted,
including escapes such as `\/` that YAML rejects. Every other file is read as
YAML, which accepts most JSON as well.

## Output

`yak template` writes YAML: source key order is preserved, hidden fields are
dropped, and documents are separated by `---`. Nothing is written unless every
document evaluates successfully.

## Grammar sketch

```
stream      := document ("---" document)* "..."?
document    := node
node        := blockMapping | blockSequence | value
blockMapping:= (key (":" | "::") value)+          -- aligned on one column
blockSequence := ("-" node)+                      -- aligned on one column
value       := literal | reference | flowSeq | flowMap
flowSeq     := "[" (value ("," value)*)? ","? "]"
flowMap     := "{" (key (":"|"::") value ("," ...)*)? "}"
key         := identifier | string | "[" value "]"
literal     := string | int | float | "true" | "false" | "null"
reference   := (dots | "$" | "$context" | "$$") postfix*
postfix     := "." identifier | "[" value "]"
dots        := "." | ".." | "..."  ...
```
