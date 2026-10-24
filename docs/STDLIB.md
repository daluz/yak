# The yak standard library

This lists the functions the language comes with; the language itself is
described in [SPEC.md](SPEC.md). Built-ins are bare names. Everything else
lives in a module whose path begins with `std` and is reached through an
[import](SPEC.md#standard-library), so growing the library cannot take a bare
name a template uses.

## Contents

- [Built-ins](#built-ins)
    - [Conversions](#conversions)
    - [Sequences](#sequences)
    - [Mappings](#mappings)
- [Modules](#modules)
    - [`std.base64`](#stdbase64)
    - [`std.cidr`](#stdcidr)
    - [`std.env`](#stdenv)
    - [`std.files`](#stdfiles)
    - [`std.gotemplate`](#stdgotemplate)
    - [`std.hash`](#stdhash)
    - [`std.json`](#stdjson)
    - [`std.math`](#stdmath)
    - [`std.path`](#stdpath)
    - [`std.random`](#stdrandom)
    - [`std.regex`](#stdregex)
    - [`std.semver`](#stdsemver)
    - [`std.strings`](#stdstrings)
    - [`std.yaml`](#stdyaml)

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
| `prune(value)` | `value` without the items and entries that are empty, at any depth. |
| `render(value, format = null)` | `value` as a string in the output format `format`, or in YAML when there is none. |
| `fail(message)` | Nothing: it stops the render with `message`. |

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

`prune` accepts anything and empties a collection from the inside out: a
sequence or mapping is pruned first and then left out if nothing remains in
it, so `prune({a: {b: null}, c: [""], d: 0})` is `{d: 0}`. The value itself
is never left out: `prune({a: null})` is `{}` and `prune(null)` is `null`.
Hidden and `:?` entries are pruned like the others and stay hidden.

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

`fail` reports `message` as an error at the position of its argument, so a
template can refuse what it cannot render and say why:

```yaml
replicas: if $$.env == "prod" then $$.replicas else fail("replicas is only set in prod")

schema Service =
  port: integer if (n) => n > 0 and n < 65536 or fail("port must be in 1..65535")
```

`message` has to be a string; [interpolation](SPEC.md#every-string-interpolates) builds one
from other values. Like any value, a call to `fail` only stops the render
when it is [evaluated](SPEC.md#failing).

The [conversions](#conversions) and the operations on
[sequences](#sequences) and [mappings](#mappings) are built-ins as well.

### Conversions

```yaml
replicas: int($$.replicas)     # "3" from the context becomes 3
port: str($$.port)             # 8080 becomes "8080"
```

| Call | Answer |
| --- | --- |
| `int(value)`, `integer(value)` | An integer, a float with no fractional part, or a string holding an integer, as an integer. |
| `float(value)` | A number, or a string holding a finite decimal number, as a float. |
| `bool(value)`, `boolean(value)` | A boolean, or the string `"true"` or `"false"`, as a boolean. |
| `str(value)`, `string(value)` | A string, number or boolean as the string that interpolating it writes. |
| `type(value)` | The name of the value's type: `"null"`, `"boolean"`, `"integer"`, `"float"`, `"string"`, `"sequence"`, `"mapping"` or `"function"`. |

A conversion is named for its type by both of the names a
[schema](SPEC.md#types) gives it, so `integer($$.replicas)` is
`int($$.replicas)`. The conversions refuse rather than guess. `int(3.9)` is an error, since
choosing a direction to round in is what [`std.math`](#stdmath) is for, and so
is `int("3.5")`. `float` refuses `"inf"`, `"nan"` and hexadecimal forms such as `"0x1p3"`,
and `bool` refuses
YAML 1.1's `yes`, `no`, `on` and `off`. `null`, a collection or a function is
an error naming its type for every conversion; `render` writes a collection
as text. `type` accepts anything, and its names are the ones diagnostics use.

### Sequences

```yaml
ports: sort(unique($$.ports))
byWeight: sort($$.services, (s) => s.weight)
privileged: some($$.ports, (p) => p < 1024)
```

| Call | Answer |
| --- | --- |
| `sort(values, key = null)` | The items in ascending order, or in that of what `key` answers for each. |
| `reverse(values)` | The items in reverse order. |
| `unique(values)` | The items without the ones equal to an item before them. |
| `flatten(values)` | The items of a sequence of sequences, in one sequence. |
| `slice(values, start, stop = null)` | The items from position `start` up to but not including `stop`, or to the end when there is no `stop`. |
| `range(start, stop = null, step = 1)` | The integers from `start` up to but not including `stop`, counting by `step`. |
| `sum(values)` | The numbers added up; `0` for `[]`. |
| `min(values, key = null)` | The item that sorts first, by its `key` when there is one. |
| `max(values, key = null)` | The item that sorts last, by its `key` when there is one. |
| `some(values, predicate = null)` | Whether `predicate` answers `true` for some item, or some item is `true`. |
| `every(values, predicate = null)` | Whether `predicate` answers `true` for every item, or every item is `true`. |

`sort`, `min` and `max` order the items, or what `key` answers for them, as
`<` does, so those must be all numbers, all strings or all sequences. A key
that answers a sequence orders by several things at once:
`sort($$.services, (s) => [s.tier, s.name])` sorts by tier, and by name within
a tier. Sorting is stable, and
of several least or greatest items `min` and `max` answer the first. `min`
and `max` of `[]` are an error. `unique` compares items with `==`, so they
must be scalars. `sum` answers an integer unless a float is among the
numbers, as `+` does.

A negative position in `slice` counts from the end, as an
[index](SPEC.md#references) does, so `slice(values, 0, 2)` is the first two
items, `slice(values, -2)` the last two, and `slice(values, 0, -1)` all but
the last. A position past either end stops there rather than failing, and a
`stop` at or before `start` gives `[]`.

`range` counts as Python's does: with one argument it is the stop and the
count starts at 0, so `range(3)` is `[0, 1, 2]` and `range(1, 4)` is
`[1, 2, 3]`. A negative `step` counts down, so `range(5, 0, -2)` is
`[5, 3, 1]`. A `stop` that `step` cannot reach gives `[]`, and a `step` of 0
is an error.

`key` and `predicate` are functions of one parameter. A `predicate` must
answer a boolean; without one, the items must be booleans. `some` and `every`
stop at the first item that decides the answer; `some([])` is `false` and
`every([])` is `true`. Whether a sequence holds an item is what
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
| `merge(x, y, deep = true, append = false)` | `x` and `y` merged as [`x << y`](SPEC.md#merging), with the choices below. |

Hidden entries count, as for `size`, and every answer keeps the mapping's
order rather than that of `keys`. A key in `keys` that `mapping` lacks is not
an error, and `pick` and `omit` keep everything else about an entry, such as
being hidden. Whether a mapping holds a key is what
[`in`](SPEC.md#operators) asks.

`visible` leaves out only entries written with `::`; a `:?` entry stays
whatever its value. `keys(visible(m))` names the keys that are not hidden.

With its defaults `merge` is `x << y`. `deep = false` lets a mapping in `y`
replace the one in `x` rather than merging into it. `append = true` joins a
key whose value is a sequence on both sides, the items of `x` first, at every
depth the merge reaches; a sequence meeting anything else is still replaced.

```yaml
merge({args: ["-v"], limits: {cpu: 1}}, {args: ["-q"], limits: {memory: 512}}, deep = false, append = true)
# {args: ["-v", "-q"], limits: {memory: 512}}
```

## Modules

Importing a module binds a mapping of its functions under the last segment of
its path, so `import std.strings` makes `strings.upper` available. Their
arguments work as for built-ins.

### `std.base64`

```yaml
import std.base64

password: base64.encode($$.password)
```

| Call | Answer |
| --- | --- |
| `encode(value)` | The standard base64 encoding of the string `value`. |
| `decode(value)` | The text that the base64 string `value` holds. |

Decoded base64 that is not UTF-8 text is an error.

### `std.cidr`

```yaml
import std.cidr

subnets: cidr.subnets($$.vpc, [8, 8, 4])  # ["10.0.0.0/24", "10.0.1.0/24", "10.0.16.0/20"]
gateway: cidr.host(cidr.subnet($$.vpc, 8, 0), 1)  # "10.0.0.1"
zones: [cidr.subnet($$.vpc, 4, i) for i in range(3)]
```

| Call | Answer |
| --- | --- |
| `host(prefix, n)` | The address `n` places into `prefix`, counting back from the end when `n` is negative, so `-1` is the last address. |
| `subnet(prefix, newbits, n)` | Subnet `n` of the subnets of `prefix` whose prefix is `newbits` longer, counting back from the end when `n` is negative. |
| `subnets(prefix, newbits)` | One subnet of `prefix` for each integer of the sequence `newbits`, that much longer, laid out one after another. |
| `split(prefix, newbits)` | Every subnet of `prefix` whose prefix is `newbits` longer, in order. |
| `network(prefix)` | `prefix` with its host bits cleared. |
| `address(prefix)` | The address that `prefix` is written with, host bits included. |
| `prefixLength(prefix)` | The length of `prefix`, as an integer. |
| `netmask(prefix)` | The mask of an IPv4 `prefix`, such as `"255.255.0.0"` for a `/16`. |
| `broadcast(prefix)` | The last address of an IPv4 `prefix`. |
| `size(prefix)` | How many addresses `prefix` holds. |
| `hosts(prefix)` | How many of those addresses are usable by hosts. |
| `contains(prefix, value)` | Whether the address or prefix `value` lies wholly inside `prefix`. |
| `overlaps(a, b)` | Whether the prefixes `a` and `b` share any address. |
| `version(value)` | `4` or `6`, for an address or a prefix. |
| `isValid(value)` | Whether the string `value` is an address or a prefix. |

Addresses and prefixes are strings. Every answer writes them in their
canonical form, so IPv6 is lowercase with its longest run of zeros shortened
to `::`. A prefix may be written with host bits set, as an interface address
such as `"10.1.2.3/16"` is; every call but `address` works from its network,
`"10.1.0.0/16"`. An IPv4-mapped address such as `"::ffff:10.0.0.1"` is IPv6.

`subnets` allocates as Terraform's `cidrsubnets` does: each subnet starts at
the first address after the one before it that is a multiple of its own size,
so `[8, 4]` in a `/16` answers a `/24` and then the `/20` that begins
`10.0.16.0`. Running out of room in `prefix` is an error. `split` answers at
most 65536 subnets, so `newbits` must be 16 or less.

`hosts` leaves out the network and broadcast addresses of an IPv4 prefix,
except that both addresses of a `/31` are usable on a point-to-point link and
a `/32` is one host. IPv6 has no broadcast address, so every address counts.
Addresses that a cloud provider reserves are for the template to subtract.

An integer can hold no more than 2^63 - 1, so `size` and `hosts` of a prefix
with more than 62 host bits, such as an IPv6 `/64`, are errors, and `n` in
`host` cannot reach past that many addresses. `netmask` and `broadcast` of an
IPv6 prefix are errors, and so are `contains` and `overlaps` between an IPv4
and an IPv6 value.

### `std.env`

```yaml
import std.env

region: env.get("AWS_REGION", "us-east-1")
```

| Call | Answer |
| --- | --- |
| `get(name, default = null)` | The value of the environment variable `name`, or `default` when it is not set. |

A variable that is set to the empty string answers `""`, not `default`. The
environment can change between renderings, so this module is only available
with [strict mode](SPEC.md#strict-mode) off.

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

In [strict mode](SPEC.md#strict-mode), `path` must stay beneath that
directory: an absolute path, one that climbs out with `..`, and a symbolic
link that leads out are errors. With strict mode off, `path` may name any
file.

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
| `nindent N TEXT` | A line break and then `indent N TEXT`, for a value that starts on a line of its own, as in `{{ .labels \| nindent 4 }}` when `labels` is YAML text from `yaml.encode`. |

There are fewer functions than, for example, Helm, because you should compute everything
else in yak and pass it in `data`, such as YAML from `std.yaml`, so the template only lays
text out.

### `std.hash`

```yaml
import { std.hash, std.yaml }

annotations:
  "checksum/config": hash.sha256(yaml.encode(.config))
```

`sha1(value)`, `sha256(value)` and `sha512(value)` answer the digest of the
string `value` in lowercase hexadecimal.

### `std.json`

```yaml
import std.json

annotation: json.encode(.config)      # '{"name":"web","ports":[80,443]}'
```

| Call | Answer |
| --- | --- |
| `encode(value, indent = 0)` | `value` as compact JSON on a single line, or indented by `indent` spaces a level when `indent` is more than 0. |
| `decode(value)` | The value that the JSON string `value` holds. |

`encode` writes what the `json` output format would, so hidden and `:?`
entries are left out and a function in `value` is an error. `decode` reads
plain JSON as a [context file](SPEC.md#context) is read, except that there
must be exactly one document, which may be any value.

### `std.math`

```yaml
import std.math

replicas: math.ceil($$.nodes / 2)
```

`abs(value)` answers the absolute value of a number. `floor(value)`,
`ceil(value)` and `round(value)` round a float down, up, or to the nearest
integer with halves away from zero, and answer an integer; an integer is
answered as it is. `round(value, digits = null)` with `digits` rounds to that
many decimal places, so `round(3.14159, 2)` is `3.14`; an integer is again
answered as it is.

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
[strict mode](SPEC.md#strict-mode) off.

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

### `std.semver`

```yaml
import std.semver

next: semver.bump($$.version, "minor")  # "1.3.0" for "1.2.3"
latest: semver.max($$.releases)  # "1.10.0", where the built-in max answers "1.9.0"
pick: semver.max($$.releases, "^1.2")
legacy: semver.satisfies($$.version, ">=1.0 <2.0")
```

| Call | Answer |
| --- | --- |
| `isValid(value)` | Whether the string `value` is a version. |
| `parse(value)` | A mapping of the `major`, `minor` and `patch` integers, and the `prerelease` and `build` sequences of identifiers. |
| `compare(a, b)` | `-1`, `0` or `1` as `a` comes before, with or after `b`. |
| `sort(values)` | The versions from lowest to highest. |
| `min(values, constraint = null)` | The lowest version, of those that meet `constraint` when there is one. |
| `max(values, constraint = null)` | The highest version, of those that meet `constraint` when there is one. |
| `bump(value, part)` | The next release that changes the `"major"`, `"minor"` or `"patch"` part of `value`. |
| `satisfies(value, constraint)` | Whether `value` meets `constraint`. |

Versions are strings written in [Semantic Versioning
2.0.0](https://semver.org), such as `"1.2.3-rc.1+build.5"`, and may start with
a lowercase `v`. Their parts may not have leading zeros, and must fit in an
integer. A string that is not a version is an error, also in a sequence.
`sort`, `min` and `max` answer the strings they were handed,
and `bump` keeps the `v` of `value`.

Versions are ordered by precedence: by `major`, `minor` and `patch`, and then
with a prerelease before its release, so `"1.0.0-rc.1"` comes before
`"1.0.0"`. Prerelease identifiers are compared one by one, numeric ones by
value and before the others, which are in ASCII order. The `v` and the build
metadata are left out, so `compare("v1.0.0+a", "1.0.0+b")` is `0`, and
`sort` keeps such versions in the order they had. `min` and `max` answer the
first of several equal versions. Without a `constraint` they are an error for
`[]`; with one they answer `null` when no version meets it.

`bump` adds one to `part`, sets the parts after it to zero, and drops the
prerelease and the build metadata, so `bump("1.2.3-rc.1", "minor")` is
`"1.3.0"`. A prerelease whose parts after `part` are already zero is a
prerelease of the release `bump` is after, so `part` stays as it is:
`bump("1.3.0-rc.1", "minor")` is `"1.3.0"` and `bump("1.2.3-rc.1", "patch")`
is `"1.2.3"`.

A constraint is one or more ranges separated by `||`, met when any of them is.
A range is one or more comparators separated by spaces or commas, met when all
of them are:

| Comparator | Meets |
| --- | --- |
| `1.2.3` or `=1.2.3` | Exactly that version. |
| `!=1.2.3` | Any other version. |
| `>1.2.3`, `>=1.2.3`, `<1.2.3`, `<=1.2.3` | The versions that compare so with it. |
| `~1.2.3` | `>=1.2.3 <1.3.0`, so patch releases. |
| `^1.2.3` | `>=1.2.3 <2.0.0`, so releases that keep the first part that is not zero: `^0.2.3` is `>=0.2.3 <0.3.0` and `^0.0.3` is `>=0.0.3 <0.0.4`. |

A comparator other than `!=` may leave out the patch, or the minor and the
patch, or write them as `x`, `X` or `*`, which stands for every value of them.
`1.2` and `1.2.x` are `>=1.2.0 <1.3.0`, `>1.2` is `>=1.3.0`, `<=1.2` is
`<1.3.0`, `~1` is `>=1.0.0 <2.0.0`, `^0.0` is `>=0.0.0 <0.1.0`, and `*` is
any version.

A prerelease meets a range only when one of its comparators names a
prerelease of the same `major.minor.patch`. So `"2.0.0-rc.2"` meets
`>=2.0.0-rc.1`, but not `>=1.0.0` or `<2.0.0`, and `"2.1.0-rc.1"` does not
meet `>=2.0.0-rc.1`. Hyphen ranges such as `1.2 - 1.4` are not supported, and
a constraint that cannot be read is an error.

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
| `fold(value)` | `value` with its lines joined as YAML's folded (`>`) block scalars join them. |

Every argument but `values`, `count` and `length` is a string, and anything
else is an error naming its type; `values` is a sequence of strings, and
`count` and `length` are integers of zero or more.
Lengths are in characters, as for `size`, so `truncate("café", 3)` is
`"caf"`.

`split` with an empty separator splits `value` into characters, and splitting
`""` gives `[]`. Without a separator, whitespace at either end gives no empty
pieces, so `split("  a  b ")` is `["a", "b"]`. `join` does not convert items,
so each must already be a string:

```yaml
import std.strings

ports: [80, 443]
list: strings.join(["\(p)" for p in .ports], ",")   # "80,443"
```

yak has no folded block scalar, since `>` is the greater-than operator;
`fold` does its work on any string, most often a `|` block. A single line
break between two lines becomes a space and each blank line between them a
line break. A line that starts with a space or a tab keeps the line breaks
around it, and the line breaks at the end of `value` are kept as they are:

```yaml
import std.strings

let text = |
  These lines are folded
  into a single one.

  This is a second paragraph.
summary: strings.fold(text)   # "These lines are folded into a single one.\nThis is a second paragraph.\n"
```

### `std.yaml`

```yaml
import std.yaml

file: yaml.encode(.config)
```

| Call | Answer |
| --- | --- |
| `encode(value)` | `value` as block YAML, ending in a newline as a file would. |
| `decode(value)` | The value that the YAML string `value` holds; `null` for an empty one. |

`encode` writes what the `yaml` output format would, so hidden and `:?`
entries are left out and a function in `value` is an error. `decode` reads
plain YAML as a [context file](SPEC.md#context) is read, except that there
must be exactly one document, which may be any value.
