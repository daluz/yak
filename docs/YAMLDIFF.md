# What changed from YAML

<p align="center">
  <img src="assets/mascot.png" alt="yak's mascot, a shaggy brown yak" width="240">
</p>

yak is laid out as YAML is, but a file that is valid YAML is not always valid
yak, and one that is may mean something else. This lists the differences;
[SPEC.md](SPEC.md) defines the language.

## Strings must be quoted

An unquoted scalar is never a string. It is an [expression](SPEC.md#values):

```yaml
a: "web"    # the string "web"
b: 42       # the integer 42
c: .a       # a reference to the sibling field "a"
d: web      # an error: unknown identifier "web"
```

So `name: hello world` is reported as a missing pair of quotes rather than
accepted. Context files are plain YAML or JSON, not yak, so an unquoted scalar
there is still the string YAML reads it as.

## Quotes and escapes

The double-quoted, single-quoted and literal block forms are all available,
but they do not behave quite as in YAML ([Strings](SPEC.md#strings)):

- Single and double quotes mean the same thing, as in Python: both process the
  backslash escapes of YAML's double-quoted strings, and both interpolate.
  They differ only in which quote has to be escaped.
- YAML's doubled quote is not an escape: `'it''s'` is an error that says to
  write `\'`.
- The folded block scalar, written `>`, is not available, since `>` is the
  greater-than operator. The error points to `fold` in
  [`std.strings`](STDLIB.md#stdstrings).

```yaml
apostrophe: "it's"
quoted: 'say "hi"'
both: 'it\'s "quoted"'
```

## Every string interpolates

Any string may contain `\( expression )`
([Interpolation](SPEC.md#interpolation)):

```yaml
name: "web"
title: "the \(.name) service"
```

Braces, `$` and every other character are plain text, so embedded templates
such as `"{{ $labels.instance }}"` pass through untouched. `\\(` writes a
literal `\(`, and a string prefixed with `r` is
[raw](SPEC.md#raw-strings): it neither interpolates nor processes escapes.

## Only `true` and `false` are booleans

`yes`, `no`, `on`, `off`, `y` and `n` are errors that say to use
`true`/`false` or add quotes.

## Anchors and YAML's tags are gone

`&anchor`, `*alias` and `!!tag` are rejected. [References](SPEC.md#references)
and [`let` bindings](SPEC.md#let-bindings) replace anchors. A `!` written
against a name is a [tag](SPEC.md#tags), which checks a value against a
schema; any other `!` is an error that points to `not`.

## Keys

Numbers and reserved words must be quoted to be used as keys, a key may be an
expression in parentheses, and duplicate keys are an error
([Keys](SPEC.md#keys)):

```yaml
"8080": "http"
($$.env + "-only"): 3
```

## Hidden and optional entries

Two separators join `:`. `::` hides an entry from the output while leaving it
visible to references ([Hidden fields](SPEC.md#hidden-fields)), and `:?`
hides it only when its value is `null`
([Optional fields](SPEC.md#optional-fields)):

```yaml
defaults::
  image: "alpine"
replicas:? $$?.replicas
```

## The merge key

YAML's `<<` survives as [the merge key](SPEC.md#the-merge-key), without an
anchor, and merges any mapping an expression can produce. Mappings held on
both sides are merged at every depth rather than replaced:

```yaml
let defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

container:
  <<: defaults
  limits:
    memory: 512     # cpu stays 1
```
