# Examples

Each file here is a working template that shows one part of the language.
They are meant to be read alongside [SPEC.md](../docs/SPEC.md), which
defines the behaviour, and rendered to see what comes out.

Build the tool from a checkout and render one from the repository root:

```console
$ go build ./cmd/yak
$ ./yak template examples/01-strings.yak
```

Notes about the language are written as `## ` comments, which yak leaves out
of the output, so what a template renders stays clean. The exception is
`09-comments.yak`, where the comments are the point.

| Example | Shows |
| --- | --- |
| [01-strings.yak](01-strings.yak) | Quoting forms, interpolation, raw strings, block scalars. |
| [02-references.yak](02-references.yak) | `.`, `..`, `$`, `$$` and `$yak`, indexing, `?.` and `??`. |
| [03-locals.yak](03-locals.yak) | `local` bindings: blocks, block values, scope, shadowing and rebinding. |
| [04-functions.yak](04-functions.yak) | Parameters, defaults, named arguments, functions as values. |
| [05-fields.yak](05-fields.yak) | Hidden `::` and optional `:?` entries, and the built-ins. |
| [06-merging.yak](06-merging.yak) | `<<` as an operator and as a key, and what `+` joins. |
| [07-conditionals.yak](07-conditionals.yak) | `if`/`then`/`else`, boolean operators, arithmetic. |
| [08-comprehensions.yak](08-comprehensions.yak) | Sequence and mapping comprehensions, filters, nesting. |
| [09-comments.yak](09-comments.yak) | Which comments reach the output and where they land. |
| [10-deployment.yak](10-deployment.yak) | All of it at once: two manifests from two context files. |

The ones that read `$context` come with the context files they expect:

```console
$ ./yak template examples/02-references.yak -c examples/02-references.ctx.yaml
$ ./yak template examples/05-fields.yak -c examples/05-fields.ctx.yaml
$ ./yak template examples/07-conditionals.yak -c examples/07-conditionals.ctx.yaml
$ ./yak template examples/08-comprehensions.yak -c examples/08-comprehensions.ctx.yaml
```

`10-deployment.yak` takes two, merged left to right, so dropping the second
renders the staging variant of the same manifests:

```console
$ ./yak template examples/10-deployment.yak \
    -c examples/10-deployment.base.yaml \
    -c examples/10-deployment.prod.yaml
```

`-f` writes another [output format](../docs/CLI.md#output-formats). Comments
reach every format that has somewhere to put them, which
`./yak template examples/09-comments.yak -f jwcc` shows, and
`-f kyaml` on the deployment writes the Kubernetes dialect from KEP-5295.
