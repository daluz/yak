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

```console
$ yak render app.yak -c production.yaml
```

## Install

```console
$ go install github.com/daluz/yak/cmd/yak@latest
```

Or build from a checkout with `go build ./cmd/yak`.

## Where to go next

- [The tour](TOUR.md) introduces the language one feature at a time.
- [The language reference](SPEC.md) defines it.
- [The standard library](STDLIB.md) lists the built-in functions and the
  `std` modules.
- [The command line](CLI.md) documents the commands, flags, context files and
  output formats.
