# The yak standard library

This lists the functions the language comes with. The language itself is
described in [SPEC.md](SPEC.md). Some functions are built in and written as
bare names; everything else lives in a module whose path begins with `std`
and is reached through an [import](SPEC.md#the-standard-library), so that
growing the library cannot take a bare name out from under a template.

## Built-ins

Built-ins are written without qualification, and they behave in every other
way like a function declared in the template: arguments go by position or by
name, a parameter written with `= default` may be left out, and a built-in is
a value that can be passed to another function. A default of `null` stands
for something the parameter cannot otherwise say, which the built-in
describes, and passing `null` means the same as leaving the argument out. A
binding of the same name [shadows](SPEC.md#standard-library) a built-in.

| Call | Answer |
| --- | --- |
| `size(value)` | How much is in a sequence, a mapping, or a string. |
| `empty(value)` | Whether the value is `null`, `{}`, `[]` or `""`. |
| `nempty(value)` | `null` when `empty(value)`, and the value itself otherwise. |
| `render(value, format = null)` | `value` as a string in the output format `format`, or in YAML when there is none. |

`size` counts items, entries, or characters:

```yaml
ports: [80, 443]
count: size(.ports)       # 2
label: size("café")       # 4, characters rather than bytes
```

Hidden entries count, because `size` describes the value rather than what the
renderer will write out. A value with nothing to measure — a number, a
boolean, `null`, or a function — is an error naming the type that was found.

`empty` and `nempty` accept anything. Only `null` and the three empty
collections are empty; `0`, `false` and `"abc"` are values like any other,
exactly as they are for `??` and `:?`. Pairing `nempty` with `:?` is how an
entry disappears when the context supplies nothing useful for it:

```yaml
labels:? nempty($$?.labels)
```

With a context of `{labels: {}}` that entry is left out, while `{labels: {app:
web}}` renders it.

`render` answers the text that rendering `value` to a file in `format` would
write, which is how a manifest holds a whole document as a string, as a
ConfigMap does:

```yaml
data:
  "app.yaml": render(.config)
  "app.json": render(.config, "json")
```

`format` is any name the `--format` flag accepts for an
[output format](CLI.md#output-formats), aliases included,
and the text ends in a newline as the file would. Hidden and `:?` entries are
left out, a function in `value` is an error, and so is a value that the format
cannot hold, such as a sequence in TOML.

The operations on [sequences](#sequences) and [mappings](#mappings) are
built-ins as well.

### Sequences

```yaml
ports: sort(unique($$.ports))
byWeight: sort($$.services, (s) => s.weight)
privileged: any($$.ports, (p) => p < 1024)
```

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
what [`in`](SPEC.md#operators) asks.

### Mappings

```yaml
names: keys($$.labels)
selector: pick($$.labels, ["app", "tier"])
```

| Call | Answer |
| --- | --- |
| `keys(mapping)` | The keys, as a sequence of strings. |
| `values(mapping)` | The values. |
| `items(mapping)` | The entries, as a sequence of `[key, value]` pairs. |
| `pick(mapping, keys)` | The entries whose key is one of `keys`. |
| `omit(mapping, keys)` | The entries whose key is not one of `keys`. |
| `visible(mapping)` | The entries that are not hidden. |

Hidden entries count, as they do for `size`, and every answer keeps the order
the entries were written in rather than that of `keys`. A key in `keys` that
`mapping` does not hold is no error, and `pick` and `omit` keep everything
else about an entry, such as being hidden. Whether a mapping holds a key is
what [`in`](SPEC.md#operators) asks.

`visible` leaves out only the entries written with `::`, and keeps the rest
as they are: an entry written with `:?` stays whatever its value. Combined
with the others, `keys(visible(m))` names the keys that are not hidden.

## Modules

Importing a module binds a mapping of its functions under the last segment of
its path, so `import std.strings` makes `strings.upper` available. The
functions take their arguments as built-ins do.

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
[context file](SPEC.md#context) is read, except that the document may be any
value rather than only a mapping, and there has to be exactly one. Bytes that
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

### `std.gotemplate`

```yaml
import { std.files, std.gotemplate }

"nginx.conf": gotemplate.render(files.read("nginx.conf.tmpl"), {
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

## Strict mode

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
