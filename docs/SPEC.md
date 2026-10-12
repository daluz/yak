# The yak language

This describes the language as it exists today. Everything that is planned
is listed in [ROADMAP.md](ROADMAP.md); using one of the
keywords held for it produces an explicit "not implemented yet" error rather
than a confusing parse failure. The tool that renders the language — its
commands, its flags and the formats it writes — is documented in
[CLI.md](CLI.md).

yak is a restricted dialect of YAML. Everything that YAML does ambiguously has
been either removed or made explicit, and a small expression language has been
added on top.

## Files

yak files use the `.yak` extension. A file holds one or more documents
separated by `---`, optionally terminated by `...`, exactly as in YAML. The
name may carry the extension of an [output format](CLI.md#output-formats)
ahead of `.yak`, as `app.toml.yak` does, which is how a template says what it
renders to.

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
single: 'and so they are here'
literal: |
  line breaks are preserved
folded: >
  these lines are
  joined with spaces
```

Unlike YAML, single and double quotes mean the same thing, as they do in
Python: both process the backslash escapes of YAML's double-quoted strings,
and both interpolate. The only difference is which quote has to be escaped,
so a string holding one kind is easiest to write in the other:

```yaml
apostrophe: "it's"
quoted: 'say "hi"'
both: 'it\'s "quoted"'
```

YAML's doubled quote is not an escape: `'it''s'` is an error that says to
write `\'` instead. Every escape that a quoted string accepts:

| Escape | Produces |
| --- | --- |
| `\n`, `\t`, `\r` | A line feed, a tab, a carriage return. |
| `\0`, `\a`, `\b`, `\v`, `\f`, `\e` | NUL, bell, backspace, vertical tab, form feed, escape. |
| `\"`, `\'`, `\\`, `\/`, `\ ` | The character itself; `\ ` is a space. |
| `\xHH`, `\uHHHH`, `\UHHHHHHHH` | A byte, or a code point, in 2, 4 or 8 hexadecimal digits. |
| `\N`, `\_`, `\L`, `\P` | Next line, non-breaking space, line separator, paragraph separator. |
| `\` at the end of a line | Nothing: the line break and the next line's indentation are dropped. |

Any other backslash sequence, such as `\d`, is an error. Block scalars have no
escapes at all, since their line breaks are written as line breaks.

Block scalars accept the usual chomping and indentation indicators: `|-`, `|+`,
`>-`, `>+`, and `|2`.

### Every string interpolates

Any string may contain `\( expression )`:

```yaml
name: "web"
title: "the \(.name) service"
url: "https://\($$.host):\($$.port)/"
```

Braces, `$` and every other character are plain text, so the templates and
queries that Kubernetes configuration carries pass through untouched:

```yaml
summary: "{{ $labels.instance }} is down"   # a Prometheus template
query: "up{job=\"api\"}"
script: |
  echo "${SHELL_VARIABLE}"
```

Write `\\(` for a literal `\(`, so `"\\(not an expression)"` renders
`\(not an expression)`. A `\(` that is never closed is an error rather than
literal text.

`\\(` is the escape in every interpolating form. In a quoted string it is the
ordinary `\\` escape followed by a `(`. Block scalars have no backslash
escapes, so there `\\(` is the only sequence a backslash changes: `\\n` in a
block scalar is still two backslashes and an `n`.

To turn interpolation off for a whole string, prefix it with `r`. Raw strings
also disable backslash escapes:

```yaml
pattern: r"^\(\d+\)$"
script: r|
  sed 's/\(v[0-9]*\)/x/'
```

A raw string is usually the better answer for text that is dense with
backslashes, such as a regular expression with groups in it.

Because a raw string has no escapes, it cannot contain its own quote: write
`r'...'` for text with a `"` in it, `r"..."` for text with a `'`, and a raw
block scalar for text with both.

### Only `true` and `false` are booleans

`yes`, `no`, `on`, `off`, `y`, and `n` are not booleans. Writing one is an
error that tells you to use `true`/`false` or add quotes.

### Anchors and tags are gone

`&anchor`, `*alias`, and `!!tag` are all rejected. Anchors are replaced by
references and `local` bindings. Tags will return with schemas. A `!` on its
own is an error too, one that points to `not`, which is how yak negates a
boolean.

YAML's `<<` survives the loss of the anchors it used to name, and merges any
mapping an expression can produce.

### Keys

A key is one of:

```yaml
plain-key: 1              # any identifier: letters, digits, _, and interior -
"a key with spaces": 2    # any quoted string, interpolation included
["computed-\($$.env)"]: 3  # an expression in brackets
```

Numbers and reserved words must be quoted to be used as keys. Duplicate keys
are an error rather than a silent overwrite.

`if`, `then`, `else`, `for`, `in`, `and`, `or` and `not` mean something
only where a value is expected, so they stay usable as bare keys: `for: 3` is
an ordinary entry.
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
    image: "\($.defaults.image):\($.defaults.tag)"
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

### Merging with `<<`

Writing `<<` where a key belongs merges the mapping on the right into the one
being written, which is what YAML's merge key does without needing an anchor
to name:

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

A mapping reads top to bottom, and that is what decides an override: the
merge overrides the entries above it, and the entries below it override the
merge. `limits` is written below, so it wins, and because both sides hold a
mapping the two merge rather than one replacing the other — `memory` comes
from the entry and `cpu` from the merge, however deep that goes. Anything
that is not a mapping on both sides is replaced outright.

A key the merge brings in takes the place the `<<` was written at, and a key
that was already there keeps the place it had. Several `<<` may appear in one
mapping, and a later one overrides an earlier one, as anything written later
does.

The modifiers mean what they do on any other entry, applied to the merge
itself. `<<::` hides everything it brings in, which leaves it readable by a
reference and out of the output. `<<:?` merges nothing when what it names is
null rather than failing, which is how an override the context may not carry
is read:

```yaml
container:
  <<:: defaults            # readable by a reference, never rendered
  <<:? $$?.overrides       # nothing to merge when the context omits it
  cpu: .limits.cpu         # 1, read out of what the merge brought in
```

`<<` has to know the keys it is merging in order to place them, so the
mapping it names is evaluated where the `<<` is written rather than when the
merged mapping is read, as is any entry of the same mapping that it decides.

`"<<"` in quotes is an ordinary key, which is how a mapping that has to carry
one is written.

## References

| Syntax | Meaning |
| --- | --- |
| `.` | The enclosing mapping. |
| `.name` | The `name` field of the enclosing mapping. |
| `..` / `..name` | The parent mapping, and its fields. |
| `...` / `...name` | The grandparent, and so on for each added dot. |
| `$` | The root of the current document. |
| `$.name` | The `name` field of that root. |
| `$$` | The merged context data. |

### Builtin variables

A name written with a leading `$` is a builtin variable. These are the only
names the language supplies; every other name is a field, a binding or a
parameter. Three of them have a sigil as a shorter alias, and the two
spellings mean exactly the same thing, so `$self.name` is `.name`:

| Variable | Alias | Meaning |
| --- | --- | --- |
| `$self` | `.` | The enclosing mapping. |
| `$root` | `$` | The root of the current document. |
| `$context` | `$$` | The merged [context](#context) data. |
| `$yak` | | Runtime information rather than the document. |

A template that refers to a mapping once may read better for saying which one
it means. `$yak` has no alias, and of the dot references only `.` has a long
spelling: walking outwards takes dots. `$super` is reserved and writing it
today is a "not implemented yet" error, and any other `$name` is an error
naming the variables that do exist.

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

`$yak` is a mapping of runtime information: it describes the rendering rather
than the document, and its fields are read like any other mapping's.

| Field | Meaning |
| --- | --- |
| `$yak.version` | The version of yak doing the rendering. |
| `$yak.filepath` | The template's path, exactly as it was given to the tool. |
| `$yak.contextpaths` | The context files, in the order they were given. |

```yaml
# rendering: yak template deploy/app.yak -c base.yaml -c prod.yaml
a: $yak.filepath            # "deploy/app.yak"
b: $yak.contextpaths[0]     # "base.yaml"
c: "built by yak \($yak.version)"
```

The path is reported as it was written rather than resolved against the
working directory, so a template says the same thing a diagnostic about it
would. A template read from standard input is called `<stdin>`, again matching
its diagnostics. `contextpaths` is a sequence that is empty rather than absent
when no context file was given, so a comprehension over it needs no guard.

This is all runtime information, so unlike `$` every document of a stream
sees the same `$yak`. An unstamped build calls itself `dev`, so `version` is
only as meaningful as the build that reads it. Writing any of these into
output makes that output depend on where the template was on disk and on
which yak rendered it, which is worth doing for a provenance note and worth
avoiding elsewhere.

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
`"schema": 1`. `import` is described in [Modules](#modules). `schema` is
described in [ROADMAP.md](ROADMAP.md), and writing it today is a "not
implemented yet" error.

## Local bindings

`local` names a value. The name is an ordinary identifier, and writing it
where a value is expected reads the binding back:

```yaml
local name = "web"
local port = 8080

service: name
url: "http://\(name):\(port)"
```

Bindings produce no output of their own. They are the replacement for YAML
anchors, and unlike a hidden field they do not have to live inside the mapping
that uses them.

Several bindings can share a `local { ... }` block. Write one per line, or
separate them with commas to fit them on one:

```yaml
local {
  region = "us-east-1"
  zone = "\(region)a"
}
local { retries = 3, timeout = 30 }
```

A binding's value may be an indented block, exactly like a mapping value:

```yaml
local defaults =
  image: "alpine"
  tag: "3.20"

image: "\(defaults.image):\(defaults.tag)"
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
local label = "\(.name)-web"
labels:
  app: label
```

A binding in an inner block shadows an outer one of the same name:

```yaml
local tier = "shared"
web:
  local tier = "frontend"
  tier: tier           # "frontend"
api:
  tier: tier           # "shared"
```

A name may also be bound again in the block it is already bound in, which
rebinds it from that line down. A reference reads the binding declared most
recently above it, and the rebinding's own value reads the binding it
replaces:

```yaml
local size = "small"
dev:
  tier: size           # "small"
local size = "\(size)-ish"
prod:
  tier: size           # "small-ish"
```

A nested block is read at the lines it is written on, which is why `dev` and
`prod` above differ. Bindings written in one `local { ... }` block count in
the order they appear. A reference written above every binding of a name
reads the first one, so a name bound once can be read anywhere in its block.

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
A rebinding names the binding it replaces rather than itself, so `local a = a`
is a circular reference while a second `local a = a` is not. A binding that is
never used is never evaluated.

## Functions

A binding whose name is followed by a parameter list is a function. Calling it
evaluates its value with the parameters bound to the arguments:

```yaml
local url(host, port) = "https://\(host):\(port)"

endpoint: url($$.host, 8080)
```

The parentheses of a call are written flush against what they call, exactly as
`.` and `[` are, and the body may be an indented block like any other binding
value:

```yaml
local endpoint(name) =
  host: "\(name).example.com"
  port: 443

web: endpoint("web")
```

A parameter may declare the value to use when a call leaves it out. Arguments
are given by position, by name, or by position and then by name:

```yaml
local url(host, port = 8080, scheme = "https") = "\(scheme)://\(host):\(port)"

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
local span(from, to = from) = "\(from)..\(to)"
```

### What a function sees

A function is a binding, so everything bindings do applies: it covers the
block it is written in, it may be used before the line that declares it, and
one in an inner block shadows an outer one of the same name. A parameter is a
binding too, and shadows anything of its name that the function could
otherwise see.

The body is evaluated where the function was written, not where it is called,
so `.name` inside one names the mapping that holds the declaration, and a name
it reads is the binding in effect at the declaration rather than at the call:

```yaml
name: "shop"
local label(suffix) = "\(.name)-\(suffix)"
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
local twice(s) = "\(s)\(s)"

v: apply(twice, "ab")   # abab
```

No output format can hold a function, so one that reaches the output is an
error rather than something rendered. Calling anything that is not a function
is an error naming what was found instead.

### Anonymous functions

A function written where a value belongs has no binding to take its name
from. Its parameters go in parentheses, and `=>` separates them from the body:

```yaml
local apply(fn, to) = fn(to)

v: apply((s) => "\(s)\(s)", "ab")   # abab
```

The parameters are the ones a binding declares, defaults included, and
everything in [What a function sees](#what-a-function-sees) holds here too:
the body is evaluated where it was written, so it reads the bindings and the
`.name` fields around it, and a function that returns one keeps the arguments
of the call that made it.

```yaml
local adder = (n) => (x) => x + n
v: adder(5)(3)                    # 8
```

The parentheses are not optional, so `s => "\(s)"` is an error rather than a
function of one parameter, and the body is a single expression. A function
whose body wants a block of its own is written as a binding, which also gives
it a name: a diagnostic about a call reports the name it was declared under,
and says where an anonymous function was written when there is none.

## Standard library

Some functions are built in. They are written without qualification, and they
behave in every other way like a function declared in the template: arguments
go by position or by name, a parameter written with `= default` may be left
out, and a built-in is a value that can be passed to another function. A
default of `null` stands for something the parameter cannot otherwise say,
which the built-in describes, and passing `null` means the same as leaving the
argument out.

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
about values in general rather than any one kind of value. Everything else
lives in a module of the [standard library](#the-standard-library) and is
reached through an import, so that growing the library cannot take a bare name
out from under a template.

### `std.strings`

```yaml
import std.strings

name: strings.lower(strings.trim($$.name))
hosts: strings.split($$.hosts, ",")
```

| Call | Answer |
| --- | --- |
| `upper(value)` | `value` in upper case. |
| `lower(value)` | `value` in lower case. |
| `trim(value, characters = null)` | `value` without any of `characters` at either end, or without whitespace when there are none. |
| `trimPrefix(value, prefix)` | `value` without `prefix` at its start, or `value` itself. |
| `trimSuffix(value, suffix)` | `value` without `suffix` at its end, or `value` itself. |
| `startsWith(value, prefix)` | Whether `value` starts with `prefix`. |
| `endsWith(value, suffix)` | Whether `value` ends with `suffix`. |
| `replace(value, old, new)` | `value` with every `old` replaced by `new`. |
| `repeat(value, count)` | `value` written `count` times over. |
| `truncate(value, length)` | The first `length` characters of `value`, or all of it when it is shorter. |
| `split(value, separator = null)` | The sequence of the pieces of `value` between each `separator`, or between runs of whitespace when there is none. |
| `join(values, separator)` | The strings of a sequence written one after another, with `separator` between them. |

Every argument but a count is a string, and anything else is an error naming
the type that was found; `count` and `length` are integers of zero or more.
Lengths are in characters rather than bytes, as they are for `size`, so
`truncate("café", 3)` is `"caf"`.

`split` with an empty separator splits `value` into its characters, and
splitting `""` gives `[]`. Without a separator, whitespace at either end is
dropped rather than giving empty pieces, so `split("  a  b ")` is
`["a", "b"]`. `join` needs every item of the sequence to be a
string already rather than converting it, so a number that ends up in the
middle of a joined string is one the template wrote there on purpose:

```yaml
ports: [80, 443]
list: strings.join(["\(p)" for p in .ports], ",")   # "80,443"
```

### `std.json`, `std.yaml` and `std.base64`

```yaml
import { std.json, std.yaml, std.base64 }

annotation: json.encode(.config)      # '{"name":"web","ports":[80,443]}'
file: yaml.encode(.config)
password: base64.encode($$.password)
```

| Call | Answer |
| --- | --- |
| `json.encode(value, indent = 0)` | `value` as compact JSON on a single line, or indented by `indent` spaces a level when `indent` is more than 0. |
| `json.decode(value)` | The value that the JSON string `value` holds. |
| `yaml.encode(value)` | `value` as block YAML, ending in a newline as a file would. |
| `yaml.decode(value)` | The value that the YAML string `value` holds; `null` for an empty one. |
| `base64.encode(value)` | The standard base64 encoding of the string `value`. |
| `base64.decode(value)` | The text that the base64 string `value` holds. |

`encode` writes what the output format of the same name would, so hidden and
`:?` entries are left out and a function in `value` is an error. `decode`
reads plain JSON or YAML rather than yak, exactly as a
[context file](#context) is read, except that the document may be any value
rather than only a mapping, and there has to be exactly one. Bytes that
`base64.decode` finds that are not UTF-8 text are an error, since a string
cannot hold them.

### `std.hash`

```yaml
import { std.hash, std.yaml }

annotations:
  "checksum/config": hash.sha256(yaml.encode(.config))
```

`sha1(value)`, `sha256(value)` and `sha512(value)` digest the string `value`,
answering the digest in lowercase hexadecimal.

### `std.seqs`

```yaml
import std.seqs

ports: seqs.sort(seqs.unique($$.ports))
byWeight: seqs.sort($$.services, (s) => s.weight)
privileged: seqs.any($$.ports, (p) => p < 1024)
```

`std.sequences` and `std.lists` are other names for the same module.

| Call | Answer |
| --- | --- |
| `sort(values, key = null)` | The items in ascending order, or in that of what `key` answers for each. |
| `reverse(values)` | The items in reverse order. |
| `unique(values)` | The items without the ones equal to an item before them. |
| `flatten(values)` | The items of a sequence of sequences, in one sequence. |
| `take(values, count)` | The first `count` items, or all of them when there are fewer. |
| `drop(values, count)` | The items after the first `count`, or `[]` when there are fewer. |
| `range(start, stop = null, step = 1)` | The integers from `start` up to but not including `stop`, counting by `step`. |
| `sum(values)` | The numbers added up; `0` for `[]`. |
| `min(values, key = null)` | The item that sorts first, by its `key` when there is one. |
| `max(values, key = null)` | The item that sorts last, by its `key` when there is one. |
| `any(values, predicate = null)` | Whether `predicate` answers `true` for some item, or some item is `true`. |
| `all(values, predicate = null)` | Whether `predicate` answers `true` for every item, or every item is `true`. |

Without a `key`, `sort`, `min` and `max` order the items themselves. Ordering
is the one `<` uses, so they need items that are all numbers or all strings,
and a `key` has to answer the same. Sorting is stable: items that sort the
same keep the order they had, and of several items that are the least or the
greatest, `min` and `max` answer the first. `unique` compares items as `==`
does, so they have to be scalars. `sum` answers an integer until a float is
among the numbers, as `+` does, and `min` and `max` of `[]` are an error.

`range` counts as Python's does. Given only one argument, that argument is
the stop and the count starts at 0, so `range(3)` is `[0, 1, 2]` and
`range(1, 4)` is `[1, 2, 3]`. A negative `step` counts down, so
`range(5, 0, -2)` is `[5, 3, 1]`, a `stop` that `step` cannot reach gives
`[]`, and a `step` of 0 is an error.

`key` and `predicate` are functions of one parameter, named or anonymous, and
a `predicate` has to answer a boolean; without one, the items themselves have
to be booleans. `any` and `all` stop at the first item that decides the
answer, so `predicate` never sees the items after it, and `any` of `[]` is
`false` while `all` of `[]` is `true`. Whether a sequence holds an item is
what [`in`](#operators) asks.

### `std.maps`

```yaml
import std.maps

names: maps.keys($$.labels)
selector: maps.pick($$.labels, ["app", "tier"])
```

`std.objects` is another name for the same module.

| Call | Answer |
| --- | --- |
| `keys(mapping)` | The keys, as a sequence of strings. |
| `values(mapping)` | The values. |
| `pick(mapping, keys)` | The entries whose key is one of `keys`. |
| `omit(mapping, keys)` | The entries whose key is not one of `keys`. |

Hidden entries count, as they do for `size`, and every answer keeps the order
the entries were written in rather than that of `keys`. A key in `keys` that
`mapping` does not hold is no error, and `pick` and `omit` keep everything
else about an entry, such as being hidden. Whether a mapping holds a key is
what [`in`](#operators) asks.

### `std.regex`

```yaml
import std.regex

tag: regex.find($$.image, r"\d+\.\d+\.\d+")
```

| Call | Answer |
| --- | --- |
| `match(value, pattern)` | Whether `pattern` matches somewhere in `value`. |
| `find(value, pattern)` | The first text that `pattern` matches, or `null`. |
| `findAll(value, pattern)` | Every text that `pattern` matches, in order. |
| `replace(value, pattern, replacement)` | `value` with every match replaced by `replacement`. |

Patterns are RE2, which matches in time linear in the input whatever the
pattern, and match anywhere unless they anchor themselves with `^` and `$`. A
`replacement` names a group of the pattern as `$1` or `${name}`, and `$$`
writes a dollar sign. A raw string keeps a pattern's backslashes as they
are, so an escaped parenthesis such as `\(` is not read as an interpolation.

### `std.math`

```yaml
import std.math

replicas: math.ceil($$.nodes / 2)
```

`abs(value)` answers the size of a number. `floor(value)`, `ceil(value)` and
`round(value)` round a float down, up, or to the nearest integer with halves
going away from zero, and answer an integer, so that the result can count
things; an integer is answered as it is. `round(value, digits = null)` given
`digits` instead rounds a float to that many places after the point and
answers a float, so `round(3.14159, 2)` is `3.14`.

### `std.path`

```yaml
import std.path

name: path.stem($$.config)                  # "nginx" for "conf/nginx.conf"
file: path.join(["conf", $$.env, "app.conf"])
```

| Call | Answer |
| --- | --- |
| `dir(value)` | Everything before the last element, or `.` when there is nothing before it. |
| `base(value)` | The last element. |
| `ext(value)` | The extension of the last element, dot included, or `""`. |
| `stem(value)` | The last element without its extension. |
| `clean(value)` | The shortest path naming the same file, with `.`, `..` and repeated slashes worked out. |
| `isAbs(value)` | Whether the path starts at the root. |
| `join(values)` | The strings of a sequence joined with slashes, cleaned, leaving out the empty ones. |

Paths are written with forward slashes on every system, which is also how
`std.files` reads them, so a template means the same file wherever it is
rendered. The answers are those of Go's `path` package: `ext("Makefile")`
is `""`, and `join(["a", "../b"])` is `"b"`.

### `std.files`

```yaml
import std.files

motd: files.read("files/motd.txt")
```

`read(path)` answers the text of the file at `path`, as it is: neither
interpolation nor anything else looks inside it. A relative path is relative
to the directory of the file whose call to `read` it is, the way an `import`
resolves, so a module reads from beside itself rather than from beside the
template importing it. Bytes that are not UTF-8 text are an error, as is a
file that is missing.

In [strict mode](#strict-mode), `path` has to stay beneath that directory: an
absolute path, one that climbs out with `..`, and a symbolic link that leads
out are all errors. Turning strict mode off lets `path` name any file,
absolute or relative.

### `std.template`

```yaml
import { std.files, std.template }

"nginx.conf": template.render(files.read("nginx.conf.tmpl"), {
  name: $$.name,
  servers: [s for s in $$.servers if s.enabled],
})
```

`render(template, data = null)` renders the string `template` as a
[Go template](https://pkg.go.dev/text/template), with `data` as its `.`, and
answers the text. It is for plain text that a yak string would be awkward to
write, such as a configuration file in another format. Go's actions are
written `{{ ... }}`, which is plain text to yak, so a template fits in an
ordinary string; one with a `\(` in it would still be interpolated by yak
first, which `r|` prevents.

`data` reaches the template as an output format would see it: hidden and `:?`
entries are left out, and a function in it is an error. Mappings arrive as Go
maps, which `range` walks in key order rather than in the order the entries
were written. A key that the data does not hold is an error rather than the
`<no value>` Go would otherwise write.

Beside Go's own functions, a template can call three of Helm's:

| Call | Answer |
| --- | --- |
| `quote VALUE` | `VALUE` written as a double-quoted string, escapes included, or `""` for `null`. |
| `indent N TEXT` | `TEXT` with `N` spaces in front of every line. |
| `nindent N TEXT` | A line break and then `indent N TEXT`, for a value that starts on a line of its own, as in `{{ .labels \| nindent 4 }}`. |

Everything else is better computed in yak and handed over in `data`, such as
YAML from `std.yaml`, so that a template only lays text out.

### `std.random`

```yaml
import std.random

suffix: random.choice(["a", "b", "c"])
```

| Call | Answer |
| --- | --- |
| `randint(min, max)` | An integer from `min` to `max`, both included. |
| `rand()` | A float from 0 up to but not including 1. |
| `choice(values)` | One item of a sequence that is not empty. |
| `shuffle(values)` | The items of a sequence in a random order. |
| `uuid()` | A random version 4 UUID. |

The answers change from one rendering to the next, so this module is only
available with [strict mode](#strict-mode) off.

### Strict mode

Rendering is strict unless the tool is told otherwise
([CLI.md](CLI.md#strict-mode)). In strict mode the same template, context and
files always render the same output, which is what makes the result safe to
review as a diff and to keep re-rendering in a GitOps loop.

Strict mode does two things. Importing a module whose answers can change from
one rendering to the next, such as `std.random`, is an error, raised when the
import is first read. And a module that can do more without strict mode does
less: `std.files` reads only beneath the directory of the file that calls it.
Both errors say that strict mode is the reason, and how to turn it off. Every
other module behaves the same in either mode.

## Modules

Every `.yak` file is a module, and a directory of them is a package. A module
is named by a dotted path that walks a directory chain and ends at the file,
so

```yaml
import mydirectory.mymodule
```

loads `./mydirectory/mymodule.yak` and binds it under the last segment of the
path. A path names a module, never a package: importing `mydirectory` itself
is an error. The binding holds the module's value, which is its one document
evaluated as it would be rendered, so it is read back with field access like
any other mapping. Hidden fields are part of that value, which is how a module
hands out a function or a default without rendering it:

```yaml
import mydirectory.mymodule

image: "\(mymodule.defaults.image):\(mymodule.defaults.tag)"
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

An import is a binding, so everything in [Scope](#scope) applies to it: it
covers the block it is written in, an inner one shadows an outer one, it can
be rebound, and a module that is never read is never loaded. The last segment
of a path has to be a name a binding can take; write `name = path` when it is
not.

A module holds exactly one document. It is evaluated against the same
context as the template that imports it, so `$$` reads the same data in both,
while `$yak.filepath` names the module's own file. A module is loaded once per
document however many times it is imported, and modules may import each other
in a cycle as long as no value ends up depending on itself.

### Resolution

A path resolves from the directory of the file that imports it, so
`mydir.module1` is `./mydir/module1.yak` beside the importing file. A path
cannot climb out of that directory.

The nearest `yak.mod` in that directory or any directory above it changes
this. It maps module paths onto other locations:

```yaml
modules:
  shared: ../shared            # a package: shared.x.y is ../shared/x/y.yak
  base: vendor/base.yak        # a module: base is vendor/base.yak
  acme: https://example.com/acme
```

A key is a dotted path, and it covers every import that the path begins,
segment by segment; when several keys do, the longest one wins. A target that
ends in `.yak` is a module and matches only its own key. Any other local
target is a package directory, and the rest of the import path is looked up
inside it. Local targets are relative to the `yak.mod` that names them. A
path that no key covers keeps resolving from the importing file's directory.

A target written as a URL names a remote package fetched over `https`, `git`
or from an OCI registry. Remote packages are **not implemented yet**: an
import that one covers is an error, and the rest of the `yak.mod` still
works. A `yak.lock` that pins remote sources will come with them.

### The standard library

Paths that begin with `std` name the standard library's modules. They never
resolve to a file and a `yak.mod` cannot map them, so a library module can
always be found under its own name, and a module added to the library cannot
take a path a project already uses. Importing one binds a mapping of its
functions under the last segment of its path, as for any other module;
[Standard library](#standard-library) lists them beside the built-ins that are
bare names. Some of them are refused or restricted in
[strict mode](#strict-mode). Rename the import to choose another namespace:

```yaml
import str = std.strings
```

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

Mappings are the one kind `+` leaves alone, because merging two of them is
`<<`, and an error says so rather than picking a depth. Adding two values of
different kinds is an error naming both, and so is adding two of a kind that
`+` does not join, such as two booleans.

`<<` merges two mappings, and the one on the right decides. A key held by
both takes its value from the right and keeps the position it has on the
left, so an override leaves the shape of the original recognisable:

```yaml
local defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

container: defaults << {limits: {memory: 512}, port: 8080}
```

That renders as:

```yaml
container:
  image: nginx
  limits:
    cpu: 1
    memory: 512
  port: 8080
```

The merge goes as deep as the two mappings do: a key whose value is a mapping
on both sides merges rather than being replaced, which is what `limits` shows
above. Anything else the right hand side holds replaces what the left one
held, so `[80] << [443]` is not what appends a port — `<<` joins nothing but
mappings, and two values of any other kind are an error naming both. An entry
also carries over whatever the mapping it came from said about it, so a `::`
on the right hides an entry the left would have rendered.

Merging builds a third mapping and leaves both operands as they were. An
entry that came through a merge still reads the mapping it was written in, so
overriding a key does not reach back into a sibling that referred to it:

```yaml
local base = {name: "web", label: "\(.name)-1"}

v: (base << {name: "api"}).label   # "web-1", not "api-1"
```

Telling a mapping from anything else means resolving the value, so a key held
by both sides is evaluated where the `<<` is written rather than where the
merged mapping is read. That is the one way `<<` is stricter than the
operators around it; what it renders is the same either way.

Written where a key belongs rather than between two values, `<<` merges into
the mapping it is written in; "Merging with `<<`" above covers that form.

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

`x in y` asks whether `y` holds `x`, and answers a boolean. `x not in y` is
its opposite, and means the same as `not (x in y)`:

| `y` is | `x in y` is true when |
| --- | --- |
| A sequence | One of its items `==` `x`. |
| A mapping | `x` is a string naming one of its keys, hidden ones included. |
| A string | `x` is a string that appears somewhere in `y`. |

Looking through a sequence asks what `==` asks, so `x` has to be a scalar,
and an item that is itself a collection is passed over rather than reported.
Anything else on the right is an error naming its type. Which substring a
string holds is the one question about strings that `in` answers; the rest
are in [`std.strings`](#stdstrings):

```yaml
web: "web" in $$.roles               # a sequence of strings
tls: "tls" in $$.listener            # a key of a mapping
secure: "https://" in $$.url         # a substring
legacy: $$.region not in ["eu", "us"]
```

`and`, `or` and `not` combine booleans, and only booleans: yak has no
truthiness, so `if 1` is an error rather than a guess. `and` and `or`
short-circuit, leaving the right hand side unevaluated whenever the left one
already decides the answer. yak spells them as words, so `&&`, `||` and `!`
are errors that name the word to write instead.

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

`not` sits below the comparisons, so it negates a whole one: `not a == b` is
`not (a == b)`, and `not x in xs` is `x not in xs`. For the same reason a
`not` cannot be the right hand side of a comparison or of arithmetic without
parentheses: `a == (not b)`. The `-` at the bottom is negation, written flush
against its operand.

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
to say how it groups: `not (if c then a else b)`. It may be spread over several
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

## Context

`$context`, or `$$` for short, is a mapping of data supplied to the rendering
from outside the template. It is read like any other mapping, it is shared by
every document of a file, and the paths it came from are
[`$yak.contextpaths`](#builtin-variables).

Context data is plain YAML or JSON rather than yak, so none of it is a
template: a `\(` in a context string is a `\(`, and an unquoted scalar there is
the string YAML reads it as. Several sources merge into the one mapping, with
a later one winning; how they are given and merged is the tool's business, and
[CLI.md](CLI.md#context-files) covers it.

Because what a context carries varies from one run to the next, a template
usually reads it through the operators for a value that may not be there:

```yaml
replicas: $$?.replicas ?? 1
selector:? $$?.selector
```

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

Comments with nothing after them come out at the end of the document.

### Comments on a binding

A `local` renders nothing of its own, so the comments written on one come out
wherever the binding is read back. That covers a comment written above a
binding, one written beside it, and the ones written inside a binding's
value:

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

Reading a binding back means writing its name as the whole value of an entry
or of a sequence item. A binding named inside a larger expression, an
interpolation among them, brings nothing along, because what comes out there
is no longer the binding's value.

The place the binding is read says its own piece first: a comment written
there comes above whatever the binding brings, and one written beside the
binding is used only where that place has none of its own. A binding read in
several places brings its comments along to each of them, and one read by a
hidden entry brings them nowhere, since `::` renders nothing at all.

### Notes about the template

A comment that starts with `## ` is left out of the output wherever it is
written. This is how to address the next person to edit the template rather
than whoever reads it, and it is how a comment on a binding says that it
describes the binding rather than the value:

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

A space has to follow the marker, so `##note` and a `### banner` are ordinary
comments, while a bare `##` is the marker with nothing written on it.

Comments in context files are not carried over at all.

## Output

A document renders in the order it was written: source key order is
preserved, comments come with the entries they were written on, hidden fields
are dropped, and so is an entry whose `:?` value turned out to be null. A
document that evaluates to `null` — one holding nothing but bindings, nothing
at all, or a body written as `null` — renders nothing, and a file of several
documents renders each of them in turn.

Which encodings that output can be written in, and how one of them is
chosen, is the tool's business: [CLI.md](CLI.md#output-formats) covers the
formats and the flags that select them. What the language guarantees is the
same in all of them — key order, hidden fields, and nothing written at all
unless every document renders successfully — and comments reach every format
that has somewhere to put them.

A value the chosen format cannot hold is an error rather than an
approximation. A function is the one value no format can hold; TOML, which
has no null, is the format that refuses the most of what the language can
otherwise say.

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
import      := "import" module | "import" "{" module+ "}"
module      := (identifier "=")? identifier ("." identifier)*
params      := "(" (param ("," param)* ","?)? ")"
param       := identifier ("=" value)?
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
loop        := "for" identifier "in" value ("if" value)?
key         := identifier | string | "[" value "]"
literal     := string | int | float | "true" | "false" | "null"
reference   := (dots | builtin | identifier) postfix*
postfix     := "?"? ("." identifier | "[" value "]") | args
args        := "(" (arg ("," arg)* ","?)? ")"
arg         := (identifier "=")? value
dots        := "." | ".." | "..."  ...
builtin     := "$" | "$$" | "$root" | "$self" | "$context" | "$yak"
```
