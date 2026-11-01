# AGENTS.md

`yak` is a templating language for structured configuration files (YAML,
JSON, TOML and others), written in Go.

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing code: it covers the
package layout, the documents that define behavior, the `make` targets, the
golden-file tests under `testdata/`, the checked documentation examples, and
the standard library conventions.

## Rules

- [docs/SPEC.md](docs/SPEC.md), [docs/STDLIB.md](docs/STDLIB.md) and
  [docs/CLI.md](docs/CLI.md) are the source of truth for behavior. When a
  language feature is added or changed, update the spec and
  [docs/TOUR.md](docs/TOUR.md) together, and run `make test-docs`.
- Prefer a `testdata/` fixture over a hand-written test, and review the diff
  `make update` produces instead of trusting it.
- Run `make check` before finishing.
- Write minimum code to solve the stated problem — no preemptive abstraction.
- Ask before assuming when requirements are ambiguous.
- Only modify files and functions directly involved in the current task.
- Clean-up rounds (inspecting code and docs for fixes and simplifications)
  follow [.agents/skills/yak.cleanup/SKILL.md](.agents/skills/yak.cleanup/SKILL.md).
- After finishing an implementation, always inform if the introduced
  modifications are a breaking change, a new feature or just a fix.
