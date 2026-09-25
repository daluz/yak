# AGENTS.md

`yak` is a YAML templating language. Go module `github.com/daluz/yak`;
the CLI entry point is `cmd/yak`, and everything else lives under `internal/`
(`lexer`, `parser`, `ast`, `eval`, `engine`, `render`, `ctxfile`, `cli`).

[docs/SPEC.md](docs/SPEC.md) defines the language and is the source of truth
for behavior.

## Commands

```console
$ make build    # build ./cmd/yak
$ make test     # go test ./...
$ make update   # rewrite golden files under testdata/
$ make check    # go vet + go test
$ make fmt      # gofmt -l -w .
```

## Tests

Most coverage is golden-file based. `testdata/template` holds a template, its
`*.ctx*.yaml` or `*.ctx*.json` context files, and the expected `*.want.yaml`.
`testdata/errors` holds a template and the exact diagnostic it should produce.
Add a fixture rather than a hand-written test when the behavior fits that
shape, and review the diff that `make update` produces instead of trusting it.
