---
name: yak.cleanup
description: >-
  Runs a round of deep code and docs inspection on the yak repository, looking
  for fixes and simplifications, then applies the ones the user picks with
  fixtures, doc updates and verification. Use when asked for a clean-up,
  an inspection round, a code or docs audit, dead-code removal, or to look for
  bugs, simplifications or stale docs and testdata.
disable-model-invocation: true
---

# Clean-up round

A clean-up has two phases: inspect and report, then fix what the user picks.
Never fix anything in the first phase.

## Phase 1: Inspect

Read the code against [docs/SPEC.md](../../../docs/SPEC.md),
[docs/STDLIB.md](../../../docs/STDLIB.md) and
[docs/CLI.md](../../../docs/CLI.md), which are the source of truth. Prove
each finding with a template, a command or a test before reporting it. Look
for these, in this order of priority:

1. **Safety**: paths that escape their directory (`..` in a module key, a
   vendored directory or a `files.read`), panics on edge inputs (the integer
   limits, empty sequences, huge counts), and loops that never end.
2. **Silent wrong output**: a value that renders differently from what the
   input said, such as numbers or timestamps reformatted out of a context
   file, defaults overwriting entries, or empty documents becoming `null`.
3. **Spec drift**: the code accepting what the spec refuses, or the other way
   round. Check the grammar section as well as the prose.
4. **Laziness**: a built-in that forces sequence items or arguments it does
   not need. `some`, `every` and `slice` must not evaluate past the items they
   use.
5. **Numbers**: integer overflow, `inf` and `nan`, and floats that should be
   integers.
6. **Packages**: `deps` and `module` behavior against the vendoring rules in
   the spec.
7. **Docs and help**: CLI help that disagrees with `docs/CLI.md`, examples
   that no longer parse or render what the text claims, and stale comments.
8. **Clean-up**: dead code, unreachable branches, duplicate helpers, fields
   nothing reads, and testdata nothing uses.

Run both checks under "Checks" below as part of the inspection.

## Report

Group the findings by the categories above. For each finding, give the
`file:line`, what goes wrong, a reproduction, and the proposed fix. Then use
AskQuestion to let the user pick which groups to fix. Ask a separate question
for every finding whose fix is a design choice. If behavior would change in a
way the spec doesn't settle, ask rather than pick.

## Phase 2: Fix

- Keep each fix minimal and inside the functions involved. Don't add
  abstractions along the way.
- Cover each behavior change with a golden fixture when it fits: put a
  template and its `.want.err` in `testdata/errors`, or a template and its
  `.want.yaml` in `testdata/template`. Use a unit test only for what a
  fixture cannot express.
- Update SPEC, STDLIB or CLI in the same change. When a language feature
  changes, update [docs/TOUR.md](../../../docs/TOUR.md) too, and render
  every example it shows.
- Delete a test when the behavior it covered is removed, and replace it with
  an error fixture when the input is now refused.

## Verify

```console
$ make fmt
$ make update
$ git diff testdata/
$ make check
```

- **Review the golden diff.** Only fixtures you meant to change may move. An
  existing `.want.err` that changes is a behavior change, so report it.
- **Sandboxes:** the `deps` and `cli` tests create git repositories and serve
  on loopback. If they fail with permission errors in a sandbox, run them
  outside it before suspecting the code.
- **Doc examples:** run the doc example check below again. It must report no
  differences except the ones the change intends.

## Final summary

Start with the outcome. Then classify every change as one of the three below,
as AGENTS.md requires:

- **Breaking**: input that used to work now fails, or now renders
  differently.
- **Feature**: something new that templates can use.
- **Fix**: everything else.

End with the findings you left unfixed and why.

## Checks

**Unused testdata.** The tests find fixtures by name, and the globs are in
`internal/engine/engine_test.go`, `internal/schemaimport/import_test.go` and
`internal/yakfmt/yakfmt_test.go`. Flag these:

- A golden or context file with no fixture beside it: a `NAME.want.*` or
  `NAME.ctx*` file without `NAME.yak`, or an import golden without its
  `.yaml` or `.json` input.
- A fixture in `testdata/errors` or `testdata/schemas` that has no
  `.want.err`.
- A helper file that no fixture in the same directory imports (as
  `lib.name`), reads (by file name), or names in a `yak.pkg`.

Two error fixtures with the same message are duplicates only if both reach
that message the same way. A nested pattern, an imported file or a built-in
argument is a different route, so keep both.

**Doc examples.** Never `git stash` to get the old build, because an
interrupted command loses the work. Export the base instead:

```sh
tmp=$(mktemp -d) && mkdir "$tmp/old" && git archive HEAD | tar -x -C "$tmp/old"
(cd "$tmp/old" && go build -o "$tmp/yak-old" ./cmd/yak) && go build -o "$tmp/yak-new" ./cmd/yak
```

Then write each ` ```yaml ` block in `docs/*.md` and `README.md` to its own
`.yak` file. Run `yak fmt` on it with both binaries, and report every block
whose exit status or stderr differs, along with its file and line.
