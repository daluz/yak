# AGENTS.md

`yak` is a YAML templating language. Go module `github.com/daluz/yak`;
the CLI entry point is `cmd/yak`, and everything else lives under `internal/`
(`token`, `lexer`, `parser`, `ast`, `eval`, `stdlib`, `module`, `deps`,
`engine`, `render`, `ctxfile`, `schemaimport`, `yakfmt`, `yakspec`, `version`,
`cli`).

A module is what an `import` names: a built-in module written in Go under
`stdlib`, or a file module read from a `.yak` template or a `.yakl` library.
A directory with a `yak.pkg` is a package; `deps` edits its dependencies,
vendors its git and OCI packages, writes its `yak.lock`, and builds and
pushes it as an OCI artifact. `module` decides which one
a path names and loads it; `eval` knows neither, and reaches modules only
through the `eval.Modules` that `engine` hands it.

These documents are the source of truth for behavior:

- [docs/SPEC.md](docs/SPEC.md): the language.
- [docs/STDLIB.md](docs/STDLIB.md): the built-in functions and the `std` modules.
- [docs/CLI.md](docs/CLI.md): the commands, flags, context files and output
  formats.

[docs/TOUR.md](docs/TOUR.md) introduces the language one feature at a time.
When a language feature is added or changed, update the tour along with the
spec, and render every example the tour shows to check its output.

## Commands

```console
$ make build # build ./cmd/yak
$ make test # go test ./...
$ make update # rewrite golden files under testdata/
$ make check # go vet + go test
$ make fmt # gofmt -l -w .
$ make docs # build the docs site with Zensical
$ make docs-serve # serve the docs site locally
```

## Tests

- Most coverage is golden-file based. `testdata/template` holds a template, its
`*.ctx*.yaml` or `*.ctx*.json` context files, and the expected `*.want.yaml`.
`testdata/errors` holds a template and the exact diagnostic it should produce.
- Add a fixture rather than a hand-written test when the behavior fits that
shape, and review the diff that `make update` produces instead of trusting it.
- To cover another output format, create an empty `NAME.want.FORMAT` beside the
template and run `make update`; use `NAME.want.FORMAT.err` where the format
has to refuse the template.

## Behaviors and Conventions

- Standard library: a built-in module under `internal/stdlib` hands a
  `module.BuiltinModule` to `module.Register` from its `init`, one file per
  module. Only the root module's built-ins are bare names; every other module
  is namespaced under `std`, so adding a function cannot take a name a
  template already uses. Names of more than one word are camelCase, and a
  built-in that takes a function calls it with `eval.Call`.
- Write minimum code to solve the stated problem — no preemptive abstraction 
- Ask before assuming when requirements are ambiguous 
- Only modify files and functions directly involved in the current task
- After finishing an implementation, always inform if the introduced
  modifications are a breaking change, a new feature or just a fix.
