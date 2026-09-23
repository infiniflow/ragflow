# RAGFlow documentation review checklist

Use this checklist for a full documentation audit or before handing off a documentation change.

## Implementation alignment

- [ ] The document presents Go as the formal implementation.
- [ ] No Python server, worker, task executor, or mixed-image path is presented as the supported deployment route.
- [ ] Python mentions are limited to an explicit Python product, client example, historical record, or required build-time helper.
- [ ] No user-facing section describes a feature as unimplemented, pending, or coming later.
- [ ] Unsupported features are omitted from user-facing instructions.

## Technical correctness

- [ ] Ports, URLs, filenames, environment variables, image names, and service order match the target contract.
- [ ] CLI syntax is confirmed in parser and dispatcher code.
- [ ] CLI behavior is confirmed in the implementation, not inferred from help text alone.
- [ ] Installer URLs and platform asset names match the checked-in scripts.
- [ ] Commands requiring a running service are not described as verified when no service was available.
- [ ] Destructive commands were not run during validation.

## Consistency and usability

- [ ] Release installation precedes source compilation for ordinary-user workflows.
- [ ] Every command is copyable and has its prerequisites stated.
- [ ] Required and optional parameters use consistent notation.
- [ ] Related documents use the same terminology, ports, defaults, and links.
- [ ] Code fences use an appropriate language.
- [ ] `git diff --check` passes.
- [ ] Only `.md`/`.mdx` files changed.
- [ ] `internal/development.md` is unchanged.
