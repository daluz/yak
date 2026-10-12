# A tour of yak

This tour introduces yak one feature at a time, starting from a file that
looks like ordinary YAML and ending with a pair of Kubernetes manifests built
from layered configuration. Every example is a complete template you can
paste into a file and render yourself.

The tour explains things by showing them. When you want the precise rules
behind a feature, [SPEC.md](SPEC.md) defines the language, and
[CLI.md](CLI.md) documents the command line tool.

- [1. Your first template](#1-your-first-template)
- [2. Values](#2-values)
- [3. Strings](#3-strings)
- [4. Mappings, sequences and keys](#4-mappings-sequences-and-keys)
- [5. References](#5-references)
- [6. The context](#6-the-context)
- [7. Hidden and optional entries](#7-hidden-and-optional-entries)
- [8. Local bindings](#8-local-bindings)
- [9. Operators](#9-operators)
- [10. Conditionals](#10-conditionals)
- [11. Functions](#11-functions)
- [12. Comprehensions](#12-comprehensions)
- [13. Built-in functions](#13-built-in-functions)
- [14. Destructuring](#14-destructuring)
- [15. Merging](#15-merging)
- [16. Comments](#16-comments)
- [17. Several documents](#17-several-documents)
- [18. Modules and the standard library](#18-modules-and-the-standard-library)
- [19. Output formats](#19-output-formats)
- [20. Putting it together](#20-putting-it-together)

## 1. Your first template

Install the tool, or build it from a checkout:

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

A yak template is a `.yak` file. Save this as `hello.yak`:

```yaml
name: "web"
replicas: 2
greeting: "hello from \(.name)"
```

and render it:

```console
$ yak render hello.yak
name: web
replicas: 2
greeting: hello from web
```

Most of that is plain YAML. The new part is `\(.name)`, which reads the
`name` entry beside it and writes it into the string. The rest of the tour is
about what can go between those parentheses, and everywhere else a value can
be written.

## 2. Values

yak has the values YAML has: strings, integers, floats, booleans, `null`,
mappings and sequences.

```yaml
name: "web"
port: 8080
ratio: 0.75
enabled: true
owner: null
```

The first rule to learn is the one that makes everything else possible:
**strings are always quoted**. An unquoted value is an expression, so
forgetting the quotes is an error rather than a string:

```yaml
name: web
```

```console
$ yak render values.yak
yak: values.yak:1:7: unknown identifier "web"; strings must be quoted, and no "local" binding is in scope here
```

YAML is generous about what counts as a boolean; yak is not. Only `true` and
`false` are booleans, and the words YAML also accepts are errors that say what
to write instead:

```yaml
enabled: yes
```

```console
yak: values.yak:1:10: "yes" is not a boolean in yak; write true, or quote it to make it a string
```

YAML's anchors (`&name`), aliases (`*name`) and tags (`!!str`) are gone too.
The features in the rest of this tour replace them.

## 3. Strings

Double and single quotes mean the same thing in yak, as they do in Python.
Both process backslash escapes, so pick whichever quote the text does not
contain:

```yaml
double: "a tab:\there"
single: 'caf\u00e9'
apostrophe: "it's"
quoted: 'say "hi"'
```

renders as:

```yaml
double: "a tab:\there"
single: café
apostrophe: it's
quoted: say "hi"
```

### Interpolation

Every string, in either quote, can hold an expression in `\( ... )`. Any
expression works there, not only a reference:

```yaml
service: "web"
port: 8080
url: "http://\(.service):\(.port)/"
next: "port \(.port + 1) is free"
helm: "{{ .Values.name }}"
literal: "\\(not interpolated)"
```

renders as:

```yaml
service: web
port: 8080
url: http://web:8080/
next: port 8081 is free
helm: '{{ .Values.name }}'
literal: \(not interpolated)
```

Braces and `$` are plain text, so Go templates, Prometheus queries and shell
variables pass through untouched. Writing `\\(` gives a literal `\(`.

### Raw strings

A string prefixed with `r` turns off both interpolation and escapes. That is
what text full of backslashes, such as a regular expression, wants:

```yaml
pattern: r"^\(\d+\)$"
```

renders as:

```yaml
pattern: ^\(\d+\)$
```

### Block scalars

YAML's block scalars work as usual: `|` keeps line breaks and `>` folds them
into spaces. They interpolate like any other string, and `r|` makes a raw one:

```yaml
service: "web"
notes: |
  \(.service) keeps its line breaks.
  This is the second line.
summary: >
  These lines are folded
  into a single one.
script: r|
  echo "${HOME} and \(this) stay as written"
```

renders as:

```yaml
service: web
notes: |
  web keeps its line breaks.
  This is the second line.
summary: |
  These lines are folded into a single one.
script: |
  echo "${HOME} and \(this) stay as written"
```

## 4. Mappings, sequences and keys

Mappings and sequences are written as in YAML, in block style or in flow
style:

```yaml
service:
  name: "web"
  ports:
    - 80
    - 443
  labels: {app: "web", tier: "frontend"}
  zones: ["a", "b"]
```

A key is a bare name, a quoted string, or an expression in brackets, which is
how a key is computed:

```yaml
plain-key: 1
"a key with spaces": 2
["port-\(80 + 1)"]: 3
for: 4
```

renders as:

```yaml
plain-key: 1
a key with spaces: 2
port-81: 3
for: 4
```

Words such as `for` and `if` have a meaning in expressions, but they stay
usable as keys. Writing the same key twice in one mapping is an error rather
than a silent overwrite.

## 5. References

You have already seen `.name`, which reads a field of the mapping it is
written in. Each extra dot walks one mapping outward, and `$` starts from the
root of the document:

```yaml
name: "shop"
metadata:
  app: ..name
  label: "\(.app)-web"
  annotations:
    root: $.name
    grandparent: ...name
```

renders as:

```yaml
name: shop
metadata:
  app: shop
  label: shop-web
  annotations:
    root: shop
    grandparent: shop
```

`.app` is a sibling of `label`, `..name` is a field of the parent mapping,
and `...name` of the grandparent. Sequences do not count as a level, so `..`
inside a sequence item still reaches the mapping that holds the sequence.
Fields and indices chain, and a negative index counts from the end:

```yaml
name: "shop"
ports:
  - name: "http"
    owner: ..name
  - name: "https"
    owner: ..name
first: $.ports[0].name
last: $.ports[-1].name
```

renders as:

```yaml
name: shop
ports:
  - name: http
    owner: shop
  - name: https
    owner: shop
first: http
last: https
```

Values are evaluated lazily, so a reference may point at something written
further down:

```yaml
alias: .name
name: "web"
```

A value that ends up depending on itself is reported as a circular reference,
along with the positions that form the cycle.

`.` and `$` have longer spellings, `$self` and `$root`, for when a template
reads better saying which mapping it means.

## 6. The context

A template usually renders data that comes from outside it: an environment
name, a replica count, a list of ports. That data is the *context*, given
with `-c` as YAML or JSON files, and read with `$$` (or `$context` in full).

Save this as `prod.yaml`:

```yaml
app: shop
env: prod
replicas: 3
```

and this as `app.yak`:

```yaml
name: "\($$.app)-web"
environment: $context.env
replicas: $$.replicas
```

```console
$ yak render app.yak -c prod.yaml
name: shop-web
environment: prod
replicas: 3
```

Several `-c` files merge into one context, with later files winning, which is
how a base configuration and an environment's overrides are layered.

The context is plain YAML, not yak: a context file needs no quotes, and
nothing in it is interpolated.

### Values that may be missing

Reading a field the context does not have is an error:

```yaml
region: $$.region
```

```console
yak: app.yak:1:9: no field "region" in mapping (the mapping is empty)
```

`?.` reads a field that may not be there, giving `null` instead of failing,
and `??` supplies a value to use in place of a `null`. They are usually
written together:

```yaml
replicas: $$?.replicas ?? 1
region: $$?.region ?? "us-east-1"
tls: $$?.tls?.enabled ?? false
```

Rendered without a context, that gives:

```yaml
replicas: 1
region: us-east-1
tls: false
```

Each `?` covers one step, so a chain that may break anywhere, like
`$$?.tls?.enabled`, needs one at every step. `?[` does the same for an index.
Only `null` triggers `??`; `false`, `0` and `""` are values like any other.

## 7. Hidden and optional entries

Writing `::` instead of `:` keeps an entry out of the output while leaving it
available to references. That is how shared defaults sit beside the data that
uses them:

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

Writing `:?` leaves an entry out only when its value is `null`, which is how
an entry the context may not supply disappears instead of rendering as
`null`:

```yaml
replicas:? $$?.replicas
selector:? $$?.selector
debug:? false
```

Rendered without a context, only `debug` remains, because `false` is not
`null`:

```yaml
debug: false
```

## 8. Local bindings

A hidden entry still belongs to a mapping. When a value just needs a name,
`local` gives it one. Bindings render nothing, and reading one back is a
matter of writing its name:

```yaml
local name = "web"
local port = 8080

service: name
url: "http://\(name):\(port)"
```

renders as:

```yaml
service: web
url: http://web:8080
```

Several bindings can share a `local { ... }` block, one per line or separated
by commas, and a binding's value may be an indented block like any mapping
value:

```yaml
local {
  region = "us-east-1"
  zone = "\(region)a"
}
local { retries = 3, timeout = 30 }

local defaults =
  image: "alpine"
  tag: "3.20"

placement: zone
client: {retries: retries, timeout: timeout}
image: "\(defaults.image):\(defaults.tag)"
```

renders as:

```yaml
placement: us-east-1a
client:
  retries: 3
  timeout: 30
image: alpine:3.20
```

### Scope

A binding is visible in the block it is written in and everything nested
inside it. One in an inner block shadows an outer one of the same name, and
one written among the entries of a mapping can read that mapping's fields:

```yaml
local tier = "shared"
web:
  local tier = "frontend"
  tier: tier
api:
  tier: tier
service:
  name: "shop"
  local label = "\(.name)-web"
  labels:
    app: label
```

renders as:

```yaml
web:
  tier: frontend
api:
  tier: shared
service:
  name: shop
  labels:
    app: shop-web
```

Like everything else, bindings are lazy: one may be used above the line that
declares it, and one that is never used is never evaluated.

Binding a name again in the same block rebinds it from that line down, and
the new value can build on the old one:

```yaml
local tag = "v1"
staging:
  image: "app:\(tag)"
local tag = "\(tag)-patched"
production:
  image: "app:\(tag)"
```

renders as:

```yaml
staging:
  image: app:v1
production:
  image: app:v1-patched
```

## 9. Operators

Arithmetic works on numbers, and `+` also joins two strings or two sequences:

```yaml
replicas: 3
memory: .replicas * 512
half: .replicas / 2
remainder: 7 % 2
spare: 10 - .replicas
negated: -.replicas
name: "shop" + "-web"
ports: [80] + [443]
```

renders as:

```yaml
replicas: 3
memory: 1536
half: 1.5
remainder: 1
spare: 7
negated: -3
name: shop-web
ports:
  - 80
  - 443
```

Two integers give an integer, except that `/` always gives a float, so
`3 / 2` is `1.5` rather than a silently truncated `1`.

Because `-` can also appear inside a name like `plain-key`, a subtraction is
written with spaces on both sides. `a-b` is one name, `a - b` is a
subtraction, and `a -b` is an error that says so:

```console
yak: app.yak:2:11: a subtraction needs whitespace on both sides of its "-"
```

Comparisons answer booleans, and `in` asks whether a sequence holds an item, a
mapping holds a key, or a string holds a substring. `and`, `or` and `not` are
spelled as words. Given a context of
`{env: prod, roles: [web, worker], url: https://shop.example.com}`:

```yaml
prod: $$.env == "prod"
big: 2 < 10
web: "web" in $$.roles
secure: "https://" in $$.url
legacy: $$.env not in ["dev", "staging"]
alerting: $$.env == "prod" and not ("batch" in $$.roles)
```

renders every entry as `true`.

## 10. Conditionals

`if ... then ... else ...` chooses between two values. Only the branch that is
chosen is evaluated, and an `else` may hold another `if` to form a chain:

```yaml
env: $$.env
replicas: if .env == "prod" then 5 else 1
tier: if .replicas > 3 then "gold" else if .replicas > 1 then "silver" else "bronze"
debug:? if .env != "prod" then true
banner:
  if .env == "prod"
  then "serving live traffic"
  else "not for production use"
```

With a context of `{env: staging}` that renders as:

```yaml
env: staging
replicas: 1
tier: bronze
debug: true
banner: not for production use
```

Leaving out the `else` gives `null` when the condition is false, and paired
with `:?` that is how an entry appears only under some condition: with
`env: prod`, `debug` would be left out entirely. A long conditional can be
spread over several lines, as `banner` is.

The condition has to be a boolean. yak has no "truthy" values, so `if 1`
is an error rather than a guess:

```console
yak: app.yak:1:7: the condition of an "if" must be a boolean, found integer
```

## 11. Functions

A binding whose name is followed by parameters is a function. A parameter
can have a default, and arguments are given by position, by name, or by
position and then by name:

```yaml
local url(host, port = 8080, scheme = "https") = "\(scheme)://\(host):\(port)"

default: url("web.example.com")
positional: url("web.example.com", 443)
named: url(port = 443, host = "api.example.com")
mixed: url("api.example.com", scheme = "http")
```

renders as:

```yaml
default: https://web.example.com:8080
positional: https://web.example.com:443
named: https://api.example.com:443
mixed: http://api.example.com:8080
```

The body of a function can be an indented block, so a function can build a
whole mapping:

```yaml
local endpoint(name) =
  host: "\(name).example.com"
  port: 443

web: endpoint("web")
api: endpoint("api")
```

renders as:

```yaml
web:
  host: web.example.com
  port: 443
api:
  host: api.example.com
  port: 443
```

A function body is evaluated where the function was written, not where it is
called. Here `.name` always means the `name` beside the declaration:

```yaml
name: "shop"
local label(suffix) = "\(.name)-\(suffix)"
labels:
  app: label("web")
  tier: label("api")
```

renders as:

```yaml
name: shop
labels:
  app: shop-web
  tier: shop-api
```

### Functions as values

A function is a value like any other: it can be passed to another function,
returned from one, or bound to a second name. A function written inline has
no name, so its parameters go in parentheses and `=>` separates them from the
body:

```yaml
local apply(fn, to) = fn(to)
local twice(s) = "\(s)\(s)"

named: apply(twice, "ab")
anonymous: apply((s) => "<\(s)>", "tag")

local adder = (n) => (x) => x + n
curried: adder(5)(3)
```

renders as:

```yaml
named: abab
anonymous: <tag>
curried: 8
```

Anonymous functions are what you will pass most often, to the built-in
functions in section 13.

## 12. Comprehensions

A comprehension builds a sequence or a mapping by walking a sequence. Write
one element followed by `for name in source`, and the element is evaluated
once per item. An `if` clause keeps only the items it accepts. Given this
context:

```yaml
host: shop.example.com
ports:
  - name: http
    number: 80
  - name: metrics
    number: 9090
```

the template

```yaml
names: ["port-\(p.name)" for p in $$.ports]
by-name: {[p.name]: p.number for p in $$.ports}
privileged: [p.name for p in $$.ports if p.number < 1024]
urls: {"\(p.name)": "http://\($$.host):\(p.number)" for p in $$.ports}
```

renders as:

```yaml
names:
  - port-http
  - port-metrics
by-name:
  http: 80
  metrics: 9090
privileged:
  - http
urls:
  http: http://shop.example.com:80
  metrics: http://shop.example.com:9090
```

A mapping comprehension takes any kind of key, and two items that produce the
same key are an error. Comprehensions nest, and the loop variable is visible
to everything inside the element:

```yaml
matrix: [["\(a)-\(b)" for b in ["east", "west"]] for a in ["eu", "us"]]
```

renders as:

```yaml
matrix:
  - - eu-east
    - eu-west
  - - us-east
    - us-west
```

## 13. Built-in functions

A handful of functions are always available without an import. `size`
measures a sequence, a mapping or a string, `empty` asks whether a value is
`null` or has nothing in it, and `nempty` turns an empty value into `null`,
which pairs with `:?`. Given a context of
`{labels: {}, annotations: {team: platform}}`:

```yaml
ports: [80, 443]
count: size(.ports)
letters: size("café")
nothing: empty("")
labels:? nempty($$?.labels)
annotations:? nempty($$?.annotations)
```

renders as:

```yaml
ports:
  - 80
  - 443
count: 2
letters: 4
nothing: true
annotations:
  team: platform
```

`size` counts characters rather than bytes, and the empty `labels` from the
context was dropped instead of rendering as `{}`.

`render` turns a value into the text an output format would write for it,
YAML unless it is given the name of another. That is what a ConfigMap wants,
since it holds whole files as strings:

```yaml
local config = {name: "web", ports: [80, 443]}

kind: "ConfigMap"
data:
  "app.yaml": render(config)
  "app.json": render(config, "json")
```

renders as:

```yaml
kind: ConfigMap
data:
  app.yaml: |
    name: web
    ports:
      - 80
      - 443
  app.json: |
    {
      "name": "web",
      "ports": [
        80,
        443
      ]
    }
```

### Sequences

`sort`, `reverse`, `unique`, `flatten`, `take`, `drop`, `range`, `sum`,
`min`, `max`, `any` and `all` work on sequences. The ones that compare or test
items accept a function, usually an anonymous one:

```yaml
local services = [
  {name: "web", weight: 3, ports: [443, 80]},
  {name: "api", weight: 1, ports: [8080]},
  {name: "jobs", weight: 2, ports: [80]},
]

ports: sort(unique(flatten([s.ports for s in services])))
by-weight: [s.name for s in sort(services, (s) => s.weight)]
heaviest: max(services, (s) => s.weight).name
total: sum([s.weight for s in services])
privileged: any(flatten([s.ports for s in services]), (p) => p < 1024)
first-two: take(range(10), 2)
countdown: range(5, 0, -2)
```

renders as:

```yaml
ports:
  - 80
  - 443
  - 8080
by-weight:
  - api
  - jobs
  - web
heaviest: web
total: 6
privileged: true
first-two:
  - 0
  - 1
countdown:
  - 5
  - 3
  - 1
```

### Mappings

`keys`, `values`, `items`, `pick`, `omit` and `visible` work on mappings:

```yaml
local labels = {app: "web", tier: "frontend", team: "platform"}

names: keys(labels)
selector: pick(labels, ["app", "tier"])
rest: omit(labels, ["team"])
pairs: items(pick(labels, ["app"]))
```

renders as:

```yaml
names:
  - app
  - tier
  - team
selector:
  app: web
  tier: frontend
rest:
  app: web
  tier: frontend
pairs:
  - - app
    - web
```

A `local` of the same name as a built-in shadows it, so a template that
already uses one of these names keeps working.

## 14. Destructuring

`items` hands out `[key, value]` pairs, and taking pairs apart is common
enough to have its own syntax. A binding can name the items of a sequence
instead of the sequence, and `_` skips an item:

```yaml
local pair = ["web", 8080]
local name, port = pair
local host, _ = ["registry.example.com", "latest"]

service: "\(name):\(port)"
registry: host
```

renders as:

```yaml
service: web:8080
registry: registry.example.com
```

The outer brackets of a pattern may be left out, so `local name, port = pair`
is the same as `local [name, port] = pair`. The sequence has to hold exactly
as many items as the pattern names.

Patterns also work in a function's parameters, where the brackets are
required, and in a comprehension's loop variable, which is where `items`
shines:

```yaml
local address([host, port], scheme = "https") = "\(scheme)://\(host):\(port)"
local limits = {cpu: 2, memory: 512}

endpoint: address(["web.example.com", 443])
doubled: {[k]: v * 2 for k, v in items(limits)}
large: [k for [k, v] in items(limits) if v > 100]
```

renders as:

```yaml
endpoint: https://web.example.com:443
doubled:
  cpu: 4
  memory: 1024
large:
  - memory
```

## 15. Merging

`<<` merges two mappings, and the one on the right wins. The merge goes as
deep as both sides hold mappings, so overriding one field of a nested mapping
keeps its siblings:

```yaml
local defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

container: defaults << {limits: {memory: 512}, port: 8080}
untouched: defaults.limits
```

renders as:

```yaml
container:
  image: nginx
  limits:
    cpu: 1
    memory: 512
  port: 8080
untouched:
  cpu: 1
  memory: 256
```

Merging builds a new mapping and leaves both sides as they were. Anything that
is not a mapping on both sides is replaced outright, so `<<` never appends to
a sequence; that is what `+` is for.

### The merge key

Written where a key belongs, `<<` merges into the mapping it is written in.
This is YAML's merge key without the anchor. A mapping reads top to bottom:
the merge overrides the entries above it, and the entries below it override
the merge:

```yaml
local defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

container:
  image: "alpine"
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

The entry modifiers work on a merge too. `<<::` brings entries in hidden, so
references can read them without them being rendered, and `<<:?` merges
nothing when the value is `null`, which is how optional overrides from the
context are read:

```yaml
local defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

sidecar:
  <<:: defaults
  <<:? $$?.overrides
  cpu: .limits.cpu
```

renders as:

```yaml
sidecar:
  cpu: 1
```

## 16. Comments

Comments are part of the output. One above an entry stays above it, and one
beside a value stays beside it. A comment that starts with `## ` is a note
for whoever edits the template, and is left out:

```yaml
# The public face of the service.
service:
  name: "web"    # and its DNS label
  ## A note for whoever edits this template next.
  port: 8080

local {
  # Where everything lands.
  region = "us-east-1"
}

placement: region
```

renders as:

```yaml
# The public face of the service.
service:
  name: web # and its DNS label
  port: 8080
# Where everything lands.
placement: us-east-1
```

A binding renders nothing of its own, so the comments written on it travel to
wherever it is read back, which is why the comment on `region` came out above
`placement`.

## 17. Several documents

A file can hold several documents separated by `---`, as in YAML. Each one has
its own bindings and its own `$`; the context is shared by all of them. Given
a context of `{app: shop}`:

```yaml
local name = "web"

kind: "Deployment"
metadata:
  name: name
---
kind: "Service"
metadata:
  name: "\($$.app)-svc"
spec:
  selector: $.metadata
```

renders as:

```yaml
kind: Deployment
metadata:
  name: web
---
kind: Service
metadata:
  name: shop-svc
spec:
  selector:
    name: shop-svc
```

A document holding nothing but bindings renders nothing at all.

## 18. Modules and the standard library

### Your own modules

Every `.yak` file is a module that another can `import`. The path is dotted
and resolves from the importing file's directory, and the module is bound
under the last segment of its path. Hidden entries are part of a module's
value, which is how a module hands out a function without rendering it.
Save this as `lib/labels.yak`:

```yaml
local team = "platform"

standard:: (app) => {app: app, team: team}
```

and import it from a template beside the `lib` directory:

```yaml
import lib.labels

metadata:
  labels: labels.standard("web")
```

which renders as:

```yaml
metadata:
  labels:
    app: web
    team: platform
```

`import shared = lib.labels` picks a different name, and an
`import { ... }` block takes several imports at once. A `yak.mod` file can
map import paths onto other directories; [SPEC.md](SPEC.md#resolution)
describes it.

### The standard library

Beyond the built-ins of section 13, the standard library lives in modules
whose paths begin with `std`: `std.strings`, `std.json`, `std.yaml`,
`std.base64`, `std.hash`, `std.regex`, `std.math`, `std.path`, `std.files`,
`std.gotemplate` and `std.random`. Given this context:

```yaml
name: "  Shop  "
hosts: a.example.com,b.example.com
image: ghcr.io/example/shop:1.4.2
```

the template

```yaml
import { std.strings, std.json, std.regex, std.hash }

local config = {name: strings.lower(strings.trim($$.name)), ports: [80, 443]}

name: config.name
hosts: strings.split($$.hosts, ",")
version: regex.find($$.image, r"\d+\.\d+\.\d+")
annotations:
  config: json.encode(config)
  checksum: hash.sha256(json.encode(config))
```

renders as:

```yaml
name: shop
hosts:
  - a.example.com
  - b.example.com
version: 1.4.2
annotations:
  config: '{"name":"shop","ports":[80,443]}'
  checksum: 6233995587e8b3290a28d0c96f35ab4ea89c5b01000efdfbd4dc39a5449923e0
```

`std.files` reads a text file from beside the template, and `std.gotemplate`
renders one as a Go template with data computed in yak, which is how a
configuration file in some other format ends up inside a value:

```yaml
import { std.files, std.gotemplate }

"nginx.conf": gotemplate.render(files.read("nginx.conf.tmpl"), {name: $$.name})
```

[STDLIB.md](STDLIB.md) lists every function in every module.

### Strict mode

Rendering is strict by default: the same template, context and files always
produce the same output. That is what keeps a rendered diff meaningful, and
what makes a template safe to re-render in a GitOps loop, where output that
changed on its own would look like a change someone made.

Only two things differ between the modes. Everything else in the language
and the library behaves the same in both.

| | Strict (the default) | `--strict=false` |
| --- | --- | --- |
| `std.random` | Importing it is an error. | Available. |
| `std.files` | Reads only beneath the directory of the file calling `read`. | Reads any path, absolute or relative. |

Since an import is a binding, and a binding that is never read is never
evaluated, the `std.random` error comes when the import is first used rather
than wherever it is written:

```console
yak: app.yak:1:8: module "std.random" can answer differently on every rendering, which strict mode does not allow; render with --strict=false to use it
```

In strict mode, `std.files` refuses an absolute path, a path that climbs out
with `..`, and a symbolic link that leads outside the directory:

```yaml
import std.files

hosts: files.read("/etc/hosts")
```

```console
yak: app.yak:3:19: "read" cannot read "/etc/hosts" in strict mode, which reads only beneath the directory of the file calling it; render with --strict=false to read anywhere
```

Both errors say that strict mode is the reason and how to turn it off.
Turning it off is a choice about the run, not the template: the same file
renders strictly again the next time it is rendered without the flag. A
template can still tell which mode it is in by reading `$yak.strict`, which is
`true` unless `--strict=false` was given.

### Information about the rendering

`$yak` is a mapping that describes the rendering itself rather than the
document, which is enough for a provenance note:

```yaml
note: "rendered from \($yak.filepath) by yak \($yak.version)"
contexts: $yak.contextpaths
```

## 19. Output formats

Everything so far rendered YAML, but the same template can render JSON,
JSONC, KYAML, TOML and more. `-f` picks one, or the name of the output file
or the template does: `app.toml.yak` renders TOML. Comments reach every
format that has room for them. The template from section 16, rendered with
`-f jsonc`:

```jsonc
{
  // The public face of the service.
  "service": {
    "name": "web", // and its DNS label
    "port": 8080
  },
  // Where everything lands.
  "placement": "us-east-1"
}
```

and with `-f toml`:

```toml
# Where everything lands.
placement = "us-east-1"

# The public face of the service.
[service]
name = "web" # and its DNS label
port = 8080
```

A format that cannot hold what the template produced refuses it rather than
guessing: JSON takes only one document, and TOML has no `null`.
[CLI.md](CLI.md#output-formats) lists every format and the flags for writing
output to files.

## 20. Putting it together

Here is a Deployment and a Service for Kubernetes, rendered from two layered
context files. The base file, `base.yaml`, describes the application:

```yaml
app: shop
component: web
env: staging

image:
  repo: ghcr.io/example/shop
  tag: "1.4.0"

ports:
  - name: http
    number: 8080

resources:
  cpu: 250m
  memory: 256Mi
```

and `prod.yaml` overrides what production changes:

```yaml
env: prod
replicas: 5

image:
  tag: 1.4.0-rc.2

ports:
  - name: http
    number: 8080
  - name: metrics
    number: 9090

annotations:
  owner: platform
```

The template, `deployment.yak`, uses most of what this tour covered:

```yaml
## Render with:
##   yak render deployment.yak -c base.yaml -c prod.yaml

local {
  name = "\($$.app)-\($$.component)"
  image = "\($$.image.repo):\($$.image.tag)"
  prod = $$.env == "prod"
}

local labels =
  app: $$.app
  component: $$.component
  env: $$.env

# Rendered by yak. Edit the template rather than this file.
apiVersion: "apps/v1"
kind: "Deployment"
metadata:
  name: name
  labels: labels
  annotations:? nempty($$?.annotations)
spec:
  replicas: if prod then $$?.replicas ?? 3 else 1
  selector:
    matchLabels: labels
  template:
    metadata:
      labels: labels
    spec:
      containers:
        - name: $$.component
          image: image
          ports: [{name: p.name, containerPort: p.number} for p in $$.ports]
          env:
            - name: "APP_ENV"
              value: $$.env
            - name: "LOG_LEVEL"
              value: if prod then "info" else "debug"
          resources:
            requests: $$.resources
            ## Production gets the headroom; everywhere else runs on what
            ## it requested.
            limits: if prod then $$.resources << {memory: "1Gi"} else $$.resources
---
local name = "\($$.app)-\($$.component)"

apiVersion: "v1"
kind: "Service"
metadata:
  name: name
  labels:
    app: $$.app
    component: $$.component
spec:
  selector: $.metadata.labels
  ports: [{name: p.name, port: p.number, targetPort: p.name} for p in $$.ports]
```

```console
$ yak render deployment.yak -c base.yaml -c prod.yaml
```

renders as:

```yaml
# Rendered by yak. Edit the template rather than this file.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop-web
  labels:
    app: shop
    component: web
    env: prod
  annotations:
    owner: platform
spec:
  replicas: 5
  selector:
    matchLabels:
      app: shop
      component: web
      env: prod
  template:
    metadata:
      labels:
        app: shop
        component: web
        env: prod
    spec:
      containers:
        - name: web
          image: ghcr.io/example/shop:1.4.0-rc.2
          ports:
            - name: http
              containerPort: 8080
            - name: metrics
              containerPort: 9090
          env:
            - name: APP_ENV
              value: prod
            - name: LOG_LEVEL
              value: info
          resources:
            requests:
              cpu: 250m
              memory: 256Mi
            limits:
              cpu: 250m
              memory: 1Gi
---
apiVersion: v1
kind: Service
metadata:
  name: shop-web
  labels:
    app: shop
    component: web
spec:
  selector:
    app: shop
    component: web
  ports:
    - name: http
      port: 8080
      targetPort: http
    - name: metrics
      port: 9090
      targetPort: metrics
```

Drop `-c prod.yaml` to render the staging variant: one replica, debug
logging, no annotations, and limits equal to the requests.

That is the whole language as it stands today. [SPEC.md](SPEC.md) has the
exact rules for everything shown here, and [ROADMAP.md](ROADMAP.md) lists
what is coming next.
