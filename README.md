# yak

<p align="center">
  <img src="docs/assets/mascot.png" alt="yak's mascot, a shaggy brown yak" width="240">
</p>

`yak` _(/jæk/)_ templatizes structured configuration files (primarily YAML, but also
JSON, TOML and others). It is similar in spirit to jsonnet, but the surface
language is closer to YAML, with the ambiguous parts removed and an expression
language added.

It was initially designed to create templates for Kubernetes manifests, Helm Chart
values files, and ArgoCD applications.

**Template: `app.yak`** <!-- example: readme template app.yak -->

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
  template:
    metadata:
      labels: $.metadata.labels
    spec:
      containers:
        - name: "web"
          image: "\(image):\(tag)"
          env:
            - name: "APP_NAME"
              value: $$.app.name
```

Save the context as `production.yaml`:

**Context file: `production.yaml`** <!-- example: readme context production.yaml -->

```yaml
app:
  name: shop
  replicas: 3
```

```console
$ yak render app.yak -c production.yaml
```

The output is a Deployment named `shop-web` with three replicas.

## Install

Building yak requires Go 1.27.1 or newer.

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

`go install` puts `yak` in `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN`
is unset. Add that directory to your `PATH` so your shell can find `yak`.

Or build from a checkout with `go build ./cmd/yak`, then run `./yak`.

## Usage

```console
$ yak render FILE.yak [-c context.yaml|context.json]... [-f FORMAT]
                        [-o FILE | -O] [--output-dir DIR]
```

```console
$ yak render app.yak -c production.yaml -f kyaml
$ yak render app.yak -c production.yaml -o rendered.json # the extension chooses JSON
$ yak render app.yak -c production.yaml -f toml -O --output-dir build
```

The last command writes `build/app.toml`.

## Where to go next

- [The tour](docs/TOUR.md) introduces the language one feature at a time.
- [What changed from YAML](docs/YAMLDIFF.md) explains the differences for YAML users.
- [The language reference](docs/SPEC.md) defines it.
- [The standard library](docs/STDLIB.md) lists the built-in functions and the
  `std` modules.
- [The command line](docs/CLI.md) documents the commands, flags, context files
  and output formats.

## Development

[CONTRIBUTING.md](CONTRIBUTING.md) covers the repository layout, the `make`
targets, and how the tests and documentation examples are checked.
