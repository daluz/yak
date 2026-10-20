# A tour of yak

This tour introduces yak one feature at a time, from a file that looks like
ordinary YAML to a pair of Kubernetes manifests built from layered
configuration. Every example is a complete template you can render yourself.
For the precise rules, [SPEC.md](SPEC.md) defines the language and
[CLI.md](CLI.md) documents the command line tool.

- [1. Your first template](#1-your-first-template)
- [2. Values](#2-values)
- [3. Strings](#3-strings)
- [4. Mappings, sequences and keys](#4-mappings-sequences-and-keys)
- [5. References](#5-references)
- [6. The context](#6-the-context)
- [7. Hidden and optional entries](#7-hidden-and-optional-entries)
- [8. Let bindings](#8-let-bindings)
- [9. Comments](#9-comments)
- [10. Several documents](#10-several-documents)
- [11. Operators](#11-operators)
- [12. Merging](#12-merging)
- [13. Conditionals](#13-conditionals)
- [14. Functions](#14-functions)
- [15. Comprehensions](#15-comprehensions)
- [16. Built-in functions](#16-built-in-functions)
- [17. Destructuring](#17-destructuring)
- [18. Modules and the standard library](#18-modules-and-the-standard-library)
- [19. Schemas](#19-schemas)
- [20. Output formats](#20-output-formats)
- [20. Packages](#20-packages)
- [21. Output formats](#21-output-formats)
- [22. Putting it together](#22-putting-it-together)

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

Most of that is plain YAML. The new part is `\(.name)`, which writes the
`name` entry beside it into the string. The rest of the tour is about what can
go between those parentheses, and anywhere else a value can be written.

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

The first rule to learn: **strings are always quoted**. An unquoted value is
an expression, so forgetting the quotes is an error:

```yaml
name: web
```

```console
$ yak render values.yak
yak: values.yak:1:7: unknown identifier "web"; strings must be quoted, and no "let" binding is in scope here
```

Only `true` and `false` are booleans. The other words YAML accepts are errors
that say what to write instead:

```yaml
enabled: yes
```

```console
yak: values.yak:1:10: "yes" is not a boolean in yak; write true, or quote it to make it a string
```

YAML's anchors (`&name`), aliases (`*name`) and tags (`!!str`) are gone too.
The features in the rest of this tour replace them, and a tag returns in
[section 19](#19-schemas) to check a value against a schema.

## 3. Strings

Double and single quotes mean the same thing, as in Python. Both process
backslash escapes, so pick whichever quote the text does not contain:

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

An `r` prefix turns off both interpolation and escapes, which suits text full
of backslashes, such as a regular expression:

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

A key is a bare name, a quoted string, which interpolates like any other, or
an expression in parentheses that gives a string:

```yaml
plain-key: 1
"a key with spaces": 2
"port-\(80 + 1)": 3
("api" + "-v2"): 4
for: 5
```

renders as:

```yaml
plain-key: 1
a key with spaces: 2
port-81: 3
api-v2: 4
for: 5
```

Words such as `for` and `if` mean something in expressions, but still work as
keys. Writing the same key twice in one mapping is an error.

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

`$` can also be spelled `$root`.

## 6. The context

Data from outside the template, such as an environment name or a list of
ports, is the *context*. It is given with `-c` as YAML or JSON files, and read
with `$$` (or `$context`, or `$ctx`, in full).

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

Several `-c` files merge into one context, later files winning, which layers
an environment's overrides on a base configuration. The context is plain
YAML, not yak: it needs no quotes, and nothing in it is interpolated.

### Values that may be missing

Reading a field the context does not have is an error:

```yaml
region: $$.region
```

```console
yak: app.yak:1:9: no field "region" in mapping (the mapping is empty)
```

`?.` reads a field that may not be there, giving `null` instead of failing,
and `??` replaces a `null`. They are usually written together:

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

`::` instead of `:` keeps an entry out of the output but available to
references, so shared defaults can sit beside the data that uses them:

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

`:?` leaves an entry out only when its value is `null`, so an entry the
context may not supply disappears instead of rendering as `null`:

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

## 8. Let bindings

A hidden entry still belongs to a mapping. When a value just needs a name,
`let` gives it one. Bindings render nothing; write the name to read one
back:

```yaml
let name = "web"
let port = 8080

service: name
url: "http://\(name):\(port)"
```

renders as:

```yaml
service: web
url: http://web:8080
```

Several bindings can share a `let { ... }` block, one per line or separated
by commas, and a binding's value may be an indented block like any mapping
value:

```yaml
let {
  region = "us-east-1"
  zone = "\(region)a"
}
let { retries = 3, timeout = 30 }

let defaults =
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
let tier = "shared"
web:
  let tier = "frontend"
  tier: tier
api:
  tier: tier
service:
  name: "shop"
  let label = "\(.name)-web"
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
let tag = "v1"
staging:
  image: "app:\(tag)"
let tag = "\(tag)-patched"
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

## 9. Comments

Comments are part of the output. One above an entry stays above it, and one
beside a value stays beside it. A comment that starts with `## ` is a note
for whoever edits the template, and is left out:

```yaml
# The public face of the service.
service:
  name: "web"    # and its DNS label
  ## A note for whoever edits this template next.
  port: 8080

let {
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

A binding renders nothing, so its comments travel to wherever it is read
back, which is why the comment on `region` came out above `placement`.

## 10. Several documents

A file can hold several documents separated by `---`, as in YAML. Each one has
its own bindings and its own `$`; the context is shared by all of them. Given
a context of `{app: shop}`:

```yaml
let name = "web"

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

## 11. Operators

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

`-` can appear inside a name like `plain-key`, so a subtraction needs spaces
on both sides. `a-b` is one name, `a - b` is a subtraction, and `a -b` is an
error that says so:

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

## 12. Merging

`<<` merges two mappings, and the one on the right wins. The merge goes as
deep as both sides hold mappings, so overriding one field of a nested mapping
keeps its siblings:

```yaml
let defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

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

Written where a key belongs, `<<` merges into the mapping it is written in,
like YAML's merge key without the anchor. The merge overrides the entries
above it, and the entries below it override the merge:

```yaml
let defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

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

The entry modifiers work on a merge too. `<<::` brings entries in hidden, for
references to read, and `<<:?` merges nothing when the value is `null`, which
suits optional overrides from the context:

```yaml
let defaults = {image: "nginx", limits: {cpu: 1, memory: 256}}

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

## 13. Conditionals

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

Without `else`, a false condition gives `null`, so with `:?` an entry appears
only under some condition: with `env: prod`, `debug` would be left out. A long
conditional can span several lines, as `banner` does.

The condition must be a boolean. yak has no "truthy" values, so `if 1` is an
error:

```console
yak: app.yak:1:7: the condition of an "if" must be a boolean, found integer
```

## 14. Functions

A binding whose name is followed by parameters is a function. A parameter
can have a default, and arguments are given by position, by name, or by
position and then by name:

```yaml
let url(host, port = 8080, scheme = "https") = "\(scheme)://\(host):\(port)"

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
let endpoint(name) =
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
let label(suffix) = "\(.name)-\(suffix)"
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

A function is a value: it can be passed to another function, returned from
one, or bound to a second name. An inline function has no name, so its
parameters go in parentheses and `=>` separates them from the body:

```yaml
let apply(fn, to) = fn(to)
let twice(s) = "\(s)\(s)"

named: apply(twice, "ab")
anonymous: apply((s) => "<\(s)>", "tag")

let adder = (n) => (x) => x + n
curried: adder(5)(3)
```

renders as:

```yaml
named: abab
anonymous: <tag>
curried: 8
```

Anonymous functions are what you will pass most often, to the built-in
functions in section 16.

## 15. Comprehensions

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
by-name: {(p.name): p.number for p in $$.ports}
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

## 16. Built-in functions

A handful of functions need no import. `size` measures a sequence, a mapping
or a string, `empty` asks whether a value is `null` or has nothing in it, and
`nempty` turns an empty value into `null`, which pairs with `:?`. `prune`
removes the empty values from inside a collection, however deep. Given a
context of
`{labels: {}, annotations: {team: platform}}`:

```yaml
ports: [80, 443]
count: size(.ports)
letters: size("café")
nothing: empty("")
labels:? nempty($$?.labels)
annotations:? nempty($$?.annotations)
metadata: prune({labels: $$.labels, owner: "", team: [$$.annotations.team, null]})
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
metadata:
  team:
    - platform
```

`size` counts characters rather than bytes, and the empty `labels` from the
context was dropped instead of rendering as `{}`, both at the top and inside
`metadata`.

`render` turns a value into the text an output format would write for it,
YAML unless another is named. A ConfigMap, which holds whole files as
strings, wants exactly that:

```yaml
let config = {name: "web", ports: [80, 443]}

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

### Conversions

`int`, `float`, `bool` and `str` move a value between the scalar types, which
a context written from environment variables, where everything is a string,
often needs. `type` names the type of a value. Given a context of
`{replicas: "3", debug: "false", port: 8080}`:

```yaml
replicas: int($$.replicas)
debug: bool($$.debug)
port: str($$.port)
ratio: float(1)
kind: type($$.replicas)
```

renders as:

```yaml
replicas: 3
debug: false
port: "8080"
ratio: 1.0
kind: string
```

The conversions are strict: `int(3.9)`, `int("3.5")` and `bool("yes")` are
errors rather than guesses, and rounding a float is left to `std.math`.

### Sequences

`sort`, `reverse`, `unique`, `flatten`, `slice`, `range`, `sum`, `min`,
`max`, `some` and `every` work on sequences. The ones that compare or test
items accept a function, usually an anonymous one:

```yaml
let services = [
  {name: "web", weight: 3, ports: [443, 80]},
  {name: "api", weight: 1, ports: [8080]},
  {name: "jobs", weight: 2, ports: [80]},
]

ports: sort(unique(flatten([s.ports for s in services])))
by-weight: [s.name for s in sort(services, (s) => s.weight)]
heaviest: max(services, (s) => s.weight).name
total: sum([s.weight for s in services])
privileged: some(flatten([s.ports for s in services]), (p) => p < 1024)
first-two: slice(range(10), 0, 2)
last-two: slice(range(10), -2)
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
last-two:
  - 8
  - 9
countdown:
  - 5
  - 3
  - 1
```

### Mappings

`keys`, `values`, `items`, `pick`, `omit` and `visible` work on mappings:

```yaml
let labels = {app: "web", tier: "frontend", team: "platform"}

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

A `let` of the same name as a built-in shadows it, so a template that
already uses one of these names keeps working.

## 17. Destructuring

`items` hands out `[key, value]` pairs, and taking them apart has its own
syntax. A binding can name the items of a sequence instead of the sequence,
and `_` skips an item:

```yaml
let pair = ["web", 8080]
let name, port = pair
let host, _ = ["registry.example.com", "latest"]

service: "\(name):\(port)"
registry: host
```

renders as:

```yaml
service: web:8080
registry: registry.example.com
```

The outer brackets of a pattern may be left out, so `let name, port = pair`
is the same as `let [name, port] = pair`. The sequence has to hold exactly
as many items as the pattern names.

Patterns also work in a function's parameters, where the brackets are
required, and in a comprehension's loop variable, which is where `items`
shines:

```yaml
let address([host, port], scheme = "https") = "\(scheme)://\(host):\(port)"
let limits = {cpu: 2, memory: 512}

endpoint: address(["web.example.com", 443])
doubled: {(k): v * 2 for k, v in items(limits)}
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

## 18. Modules and the standard library

### Your own modules

Every `.yak` file is a module that another can `import`, and so is every
`.yakl` file: a library, which is only ever imported, so rendering one is an
error. Hidden entries are part of a module's value, which is how a module
hands out a function without rendering it. Save this as `lib/labels.yakl`:

```yaml
let team = "platform"

standard:: (app) => {app: app, team: team}
```

The import path is dotted and resolves from the importing file's directory,
and the module is bound under the last segment of its path. Import it from a
template beside `lib/`:

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
`import { ... }` block takes several imports at once.

### The standard library

Beyond the built-ins of section 16, the standard library lives in modules
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

let config = {name: strings.lower(strings.trim($$.name)), ports: [80, 443]}

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
renders one as a Go template with data computed in yak, so a configuration
file in another format can end up inside a value:

```yaml
import { std.files, std.gotemplate }

"nginx.conf": gotemplate.render(files.read("nginx.conf.tmpl"), {name: $$.name})
```

[STDLIB.md](STDLIB.md) lists every function in every module.

### Strict mode

Rendering is strict by default: the same template, context and files always
produce the same output, so a rendered diff stays meaningful and a template is
safe to re-render in a GitOps loop. Only two things differ between the modes:

| | Strict (the default) | `--strict=false` |
| --- | --- | --- |
| `std.random` | Importing it is an error. | Available. |
| `std.files` | Reads only beneath the directory of the file calling `read`. | Reads any path, absolute or relative. |

An import is a binding, so the `std.random` error comes when the import is
first used rather than where it is written:

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

Both errors say how to turn strict mode off. That is a choice about the run,
not the template, and a template can read it as `$yak.strict`.

## 19. Schemas

A schema says what a value may hold, and fills in the fields a mapping leaves
out. `schema` declares one, and a tag, `!Name`, checks a value against it:

```yaml
schema Port =
  name: string
  number: integer
  protocol: "TCP" | "UDP" = "TCP"

ports:
  - !Port {name: "http", number: 8080}
  - !Port {name: "dns", number: 53, protocol: "UDP"}
```

renders as:

```yaml
ports:
  - name: http
    number: 8080
    protocol: TCP
  - name: dns
    number: 53
    protocol: UDP
```

`"TCP" | "UDP"` takes either string, and `= "TCP"` is the default for a port
that leaves it out. A value that does not fit is an error at the value, as
when the first port is written `{name: "https", number: "443"}`:

```console
$ yak render ports.yak
yak: ports.yak:7:35: field "number" of schema "Port" expects integer, found "443"
```

So is a field the schema does not declare, which catches a misspelling such
as `{name: "ssh", numbr: 22}`:

```console
yak: ports.yak:7:32: schema "Port" has no field "numbr"; it declares "name", "number", "protocol"
```

### Defaults, checks and optional fields

A default can read the mapping it fills in through `.`, `if` adds a check to
a type, `(string): string` types the keys of a mapping that declares none,
and `:?` marks a field that may be left out:

```yaml
let positive = (n) => n > 0

schema Service =
  name: string
  replicas: integer if positive = 1
  host: string = "\(.name).svc.cluster.local"
  labels: {(string): string} = {}
  selector:? mapping

web: !Service
  name: "web"
  replicas: 3
api: !Service {name: "api", labels: {team: "platform"}}
```

renders as:

```yaml
web:
  name: web
  replicas: 3
  host: web.svc.cluster.local
  labels: {}
api:
  name: api
  labels:
    team: platform
  replicas: 1
  host: api.svc.cluster.local
```

The defaults follow the fields that were written. A tag that ends its line
applies to the block below it, as `web` shows.

### Open schemas

A schema is closed: every key needs a field. `schema*` declares an open one,
which lets other keys through. `<<` merges another schema's fields in, and a
`schema { ... }` block declares several at once:

```yaml
schema {
  Tier = "gold" | "silver"
  Service = {name: string, tier: Tier = "silver"}
}
schema* LooseService =
  <<: Service

strict: !Service {name: "web", tier: "gold"}
loose: !LooseService {name: "batch", schedule: "@daily"}
```

renders as:

```yaml
strict:
  name: web
  tier: gold
loose:
  name: batch
  schedule: '@daily'
  tier: silver
```

### Checking the context

`schema $$` checks the context before anything reads it, and the defaults it
fills in are what `$$` answers everywhere. An open one leaves room for the
fields it does not mention:

```yaml
schema* $$ =
  env: "dev" | "prod"
  replicas: integer = 1

env: $$.env
replicas: $$.replicas
```

With a context of `{env: prod}` that renders as:

```yaml
env: prod
replicas: 1
```

and a context file that gets it wrong is reported against the schema:

```console
$ yak render app.yak -c qa.yaml
yak: app.yak:1:9: field "env" of the context expects "dev" or "prod", found "qa"
```

`schema $` checks the document itself the same way.

### Schemas from a module

The schemas at the top of a module are there for whoever imports it. Save
this as `lib/k8s.yakl`:

```yaml
schema Metadata =
  name: string
  labels: {(string): string} = {}

schema Deployment =
  metadata: Metadata
  image: string
  replicas: integer = 1
```

and tag with the module's name in front:

```yaml
import lib.k8s

web: !k8s.Deployment
  metadata: {name: "web"}
  image: "nginx:1.27"
```

which renders as:

```yaml
web:
  metadata:
    name: web
    labels: {}
  image: nginx:1.27
  replicas: 1
```

The nested `metadata` is checked against `Metadata` and gets its defaults
too. [The spec](SPEC.md#schemas) has every type and the exact rules of when
a value is checked.

## 20. Packages

A directory with a `yak.pkg` at its root is a package, which is how
libraries such as `lib/k8s.yakl` are shared. `yak init` makes the current
directory one, with a `yak.pkg` like this:

```yaml
package:
  name: web
  version: 0.1.0
dependencies: {}
```

Inside a package an import path resolves from the package root rather than
from the importing file, so every file in the package names a library the
same way. The `dependencies` of a `yak.pkg` map import paths onto other
packages; [SPEC.md](SPEC.md#packages) describes them. A package can come
from a git repository or an OCI registry:

```console
$ yak add git+https://github.com/acme/lib.git@v1.2.0
$ yak add oci://ghcr.io/acme/labels:2.0.0
```

adds each under the name its own `yak.pkg` gives it, vendors it into
`vendor/` and pins its commit or digest in `yak.lock`. After a fresh checkout,
`yak sync` vendors the same ones again. `yak package build` and
`yak package push oci://ghcr.io/acme/web` publish a package for others to
add.

`yak render` given the package directory renders every template in it, and
leaves the libraries out:

```console
$ yak render . --output-dir build
```

## 21. Output formats

The same template can render JSON, JSONC, KYAML, TOML and more. `-f` picks
one, or else the name of the output file or the template does:
`app.toml.yak` renders TOML. Comments reach every format with room for them.
The template from [section 9](#9-comments), rendered with `-f jsonc`:

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

A template can tell which format it is rendering from `$yak.format`, which
is always the format's own name: `-f hujson` gives `jwcc`, and an output
file ending in `.yml` gives `yaml`.

```yaml
note: "rendered as \($yak.format)"
```

rendered with `-f hujson`:

```jsonc
{
  "note": "rendered as jwcc",
}
```

A format that cannot hold what the template produced refuses it: JSON takes
only one document, and TOML has no `null`. [CLI.md](CLI.md#output-formats)
lists every format and the flags for writing output to files.

### Information about the rendering

`$yak.format` is one of the builtin variables that describe the rendering
rather than the document. `$filepath`, `$contextpaths`, `$workdir` and the
rest of `$yak` are others, which is enough for a provenance note:

```yaml
note: "rendered from \($filepath) by yak \($yak.version)"
contexts: $contextpaths
```

[The spec](SPEC.md#builtin-variables) lists every builtin variable.

## 22. Putting it together

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

schema* $$ =
  app: string
  component: string
  env: "staging" | "prod"
  replicas: integer = 3
  ports: [{name: string, number: integer}]

let {
  name = "\($$.app)-\($$.component)"
  image = "\($$.image.repo):\($$.image.tag)"
  prod = $$.env == "prod"
}

let labels =
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
  replicas: if prod then $$.replicas else 1
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
let name = "\($$.app)-\($$.component)"

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
logging, no annotations, and limits equal to the requests. A context whose
`env` is neither `staging` nor `prod`, or whose port has no `number`, is
refused by the schema before anything renders.

That is the whole language as it stands today. [SPEC.md](SPEC.md) has the
exact rules for everything shown here, and [ROADMAP.md](ROADMAP.md) lists
what is coming next.
