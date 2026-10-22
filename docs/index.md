# yak

`yak` _(/jæk/)_ templatizes structured configuration files (primarily YAML, but also
JSON, TOML and others). It is similar in spirit to jsonnet, but the surface
language is closer to YAML, with the ambiguous parts removed and an expression
language added.

It was initially designed to create templates for Kubernetes manifests, Helm Chart
values files, and ArgoCD applications.

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
