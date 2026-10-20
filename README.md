# yak

`yak` templatizes YAML. It is similar in spirit to jsonnet, but the surface
language is YAML itself, with the ambiguous parts removed and a small
expression language added.

```yaml
# app.yak
let image = "alpine"
let tag = "3.20"

apiVersion: "apps/v1"
kind: "Deployment"
metadata:
  name: "\($$.app.name)-web"
  labels:
    app: ..name
spec:
  replicas: $$.app?.replicas ?? 1
  selector:
    matchLabels: $.metadata.labels
  containers:
    - image: "\(image):\(tag)"
      env:
        - name: "APP_NAME"
          value: $$.app.name
```

```console
$ yak render app.yak -c production.yaml
```

## Install

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

Or build from a checkout with `go build ./cmd/yak`.

## Usage

```console
$ yak render FILE.yak [-c context.yaml|context.json]... [-f FORMAT]
                        [-o FILE | -O] [--output-dir DIR]
```

```console
$ yak render app.yak -c production.yaml -f kyaml
$ yak render app.yak -o rendered.json            # the extension chooses JSON
$ yak render app.toml.yak -O --output-dir build  # writes build/app.toml
```

## Where to go next

- [The tour](docs/TOUR.md) introduces the language one feature at a time.
- [The language reference](docs/SPEC.md) defines it.
- [The standard library](docs/STDLIB.md) lists the built-in functions and the
  `std` modules.
- [The command line](docs/CLI.md) documents the commands, flags, context files
  and output formats.

## Development

[AGENTS.md](AGENTS.md) lists the `make` targets and how the golden-file tests
under `testdata/` are laid out.
