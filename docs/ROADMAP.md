# Roadmap

Everything [SPEC.md](SPEC.md), [STDLIB.md](STDLIB.md) and [CLI.md](CLI.md)
describe is implemented. What follows is the planned order of the remaining
work.

## 1. `fmt`

`yak fmt` formats yak files following best practices. It slots into
`internal/cli` beside `render`, and can reuse `internal/engine`.

## 2. More `schema import` formats

`schema import` reads Kubernetes CustomResourceDefinitions. JSON Schema and
OpenAPI v3 documents are to follow, each as a value of `--format` that
`schemaimport.Detect` recognizes; `internal/schemaimport` translates an
OpenAPI schema node already, and `crd.go` is what is CRD-specific.
