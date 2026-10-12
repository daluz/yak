# The yak standard library

This lists the functions the language comes with; the language itself is
described in [SPEC.md](SPEC.md). Built-ins are bare names. Everything else
lives in a module whose path begins with `std` and is reached through an
[import](SPEC.md#standard-library), so growing the library cannot take a bare
name a template uses.

## Built-ins

Built-ins behave like functions declared in the template: arguments go by
position or by name, a parameter with `= default` may be left out, and a
built-in is a value that can be passed to another function. A default of
`null` stands for a behavior the built-in describes, and passing `null` is the
same as leaving the argument out. A binding of the same name
[shadows](SPEC.md#standard-library) a built-in.

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

Hidden entries count, since `size` measures the value, not its output. A
number, boolean, `null` or function is an error naming its type.

`empty` and `nempty` accept anything. Only `null` and the three empty
collections are empty; `0`, `false` and `"abc"` are not, as for `??` and
`:?`. Paired with `:?`, `nempty` drops an entry when the context supplies
nothing useful for it:

```yaml
labels:? nempty($$?.labels)
```

With a context of `{labels: {}}` that entry is left out, while `{labels: {app:
web}}` renders it.

`render` answers the text that rendering `value` to a file in `format` would
write, so a manifest such as a ConfigMap can hold a whole document as a
string:

```yaml
data:
  "app.yaml": render(.config)
  "app.json": render(.config, "json")
```

`format` is any name `--format` accepts for an
[output format](CLI.md#output-formats), aliases included, and the text ends in
a newline as the file would. Unlike a null document, a `null` value is not
left out: `render(null)` is `"null\n"`. Hidden and `:?` entries are left out,
and a function in `value`, or a value the format cannot hold such as a
sequence in TOML, is an error.

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

`sort`, `min` and `max` order the items, or what `key` answers for them, as
`<` does, so those must be all numbers or all strings. Sorting is stable, and
of several least or greatest items `min` and `max` answer the first. `min`
and `max` of `[]` are an error. `unique` compares items with `==`, so they
must be scalars. `sum` answers an integer unless a float is among the
numbers, as `+` does.

`range` counts as Python's does: with one argument it is the stop and the
count starts at 0, so `range(3)` is `[0, 1, 2]` and `range(1, 4)` is
`[1, 2, 3]`. A negative `step` counts down, so `range(5, 0, -2)` is
`[5, 3, 1]`. A `stop` that `step` cannot reach gives `[]`, and a `step` of 0
is an error.

`key` and `predicate` are functions of one parameter. A `predicate` must
answer a boolean; without one, the items must be booleans. `any` and `all`
stop at the first item that decides the answer; `any([])` is `false` and
`all([])` is `true`. Whether a sequence holds an item is what
[`in`](SPEC.md#operators) asks.

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

Hidden entries count, as for `size`, and every answer keeps the mapping's
order rather than that of `keys`. A key in `keys` that `mapping` lacks is not
an error, and `pick` and `omit` keep everything else about an entry, such as
being hidden. Whether a mapping holds a key is what
[`in`](SPEC.md#operators) asks.

`visible` leaves out only entries written with `::`; a `:?` entry stays
whatever its value. `keys(visible(m))` names the keys that are not hidden.

## Modules

Importing a module binds a mapping of its functions under the last segment of
its path, so `import std.strings` makes `strings.upper` available. Their
arguments work as for built-ins.

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

Every argument but `count` and `length` is a string, and anything else is an
error naming its type; `count` and `length` are integers of zero or more.
Lengths are in characters, as for `size`, so `truncate("café", 3)` is
`"caf"`.

`split` with an empty separator splits `value` into characters, and splitting
`""` gives `[]`. Without a separator, whitespace at either end gives no empty
pieces, so `split("  a  b ")` is `["a", "b"]`. `join` does not convert items,
so each must already be a string:

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
reads plain JSON or YAML as a [context file](SPEC.md#context) is read, except
that there must be exactly one document, which may be any value. Decoded
base64 that is not UTF-8 text is an error.

### `std.hash`

```yaml
import { std.hash, std.yaml }

annotations:
  "checksum/config": hash.sha256(yaml.encode(.config))
```

`sha1(value)`, `sha256(value)` and `sha512(value)` answer the digest of the
string `value` in lowercase hexadecimal.

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

Patterns are RE2, which matches in linear time, and match anywhere unless
anchored with `^` and `$`. A `replacement` names a group as `$1` or
`${name}`, and `$$` writes a dollar sign. Write patterns as raw strings, so
that an escaped parenthesis such as `\(` is not read as an interpolation.

### `std.math`

```yaml
import std.math

replicas: math.ceil($$.nodes / 2)
```

`abs(value)` answers the absolute value of a number. `floor(value)`,
`ceil(value)` and `round(value)` round a float down, up, or to the nearest
integer with halves away from zero, and answer an integer; an integer is
answered as it is. `round(value, digits = null)` with `digits` rounds to that
many decimal places and answers a float, so `round(3.14159, 2)` is `3.14`.

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

Paths use forward slashes on every system, as `std.files` does, so a template
means the same file wherever it is rendered. The answers are those of Go's
`path` package: `ext("Makefile")` is `""`, and `join(["a", "../b"])` is
`"b"`.

### `std.files`

```yaml
import std.files

motd: files.read("files/motd.txt")
```

`read(path)` answers the text of the file at `path` as it is, with no
interpolation. A relative path is relative to the directory of the file that
calls `read`, as an `import` resolves, so a module reads from beside itself
rather than beside the template importing it. A missing file, or one that is
not UTF-8 text, is an error.

In [strict mode](#strict-mode), `path` must stay beneath that directory: an
absolute path, one that climbs out with `..`, and a symbolic link that leads
out are errors. With strict mode off, `path` may name any file.

### `std.gotemplate`

```yaml
import { std.files, std.gotemplate }

"nginx.conf": gotemplate.render(files.read("nginx.conf.tmpl"), {
  name: $$.name,
  servers: [s for s in $$.servers if s.enabled],
})
```

`render(template, data = null)` renders the string `template` as a
[Go template](https://pkg.go.dev/text/template) with `data` as its `.`, and
answers the text. It suits plain text that a yak string would make awkward,
such as a configuration file in another format. Go's `{{ ... }}` actions are
plain text to yak, but a `\(` in the template is still interpolated by yak
first unless the string is raw, as with `r|`.

`data` reaches the template as an output format would see it: hidden and `:?`
entries are left out, and a function in it is an error. Mappings arrive as Go
maps, which `range` walks in key order rather than source order. A key the
data does not hold is an error rather than Go's `<no value>`.

Besides Go's `text/template` functions, the following extra template functions are available:

| Call | Answer |
| --- | --- |
| `quote VALUE` | `VALUE` written as a double-quoted string, escapes included, or `""` for `null`. |
| `indent N TEXT` | `TEXT` with `N` spaces in front of every line. |
| `nindent N TEXT` | A line break and then `indent N TEXT`, for a value that starts on a line of its own, as in `{{ .labels \| nindent 4 }}`. |

There are fewer functions than, for example, Helm, because you should compute everything
else in yak and pass it in `data`, such as YAML from `std.yaml`, so the template only lays
text out. Other functions might be added in the future if they prove to be helpful.

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

The answers change between renderings, so this module is only available with
[strict mode](#strict-mode) off.

## Strict mode

Rendering is strict unless the tool is told otherwise
([CLI.md](CLI.md#strict-mode)). In strict mode the same template, context and
files always render the same output, so the result is safe to review as a
diff and to re-render in a GitOps loop.

Strict mode does two things. Importing a module whose answers can change
between renderings, such as `std.random`, is an error, raised when the import
is first read. And `std.files` reads only beneath the directory of the file
that calls it. Both errors name strict mode as the reason and say how to turn
it off. Every other module behaves the same in either mode. A template reads
the mode as [`$yak.strict`](SPEC.md#builtin-variables).
