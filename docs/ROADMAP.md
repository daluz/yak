# Roadmap

Everything [SPEC.md](SPEC.md), [STDLIB.md](STDLIB.md) and [CLI.md](CLI.md)
describe is implemented. What follows is the planned order of the remaining
work.

## 1. `fmt`

`yak fmt` formats yak files following best practices. It slots into
`internal/cli` beside `render`, and can reuse `internal/engine`.
