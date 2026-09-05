### Summary

Describe the concrete problem and resulting behavior, including what users observe.

### Scope and architecture

- Affected module(s); extension / local core change / upstream integration:
- Applicable [rule IDs](../docs/develop/architecture-checks-ru.md) and transition stage, if any:
- For a core change: why it is needed, integration contract, and accepted upstream commit when verified:
- Removed obsolete code, registrations or dependencies, if applicable:

### Verification

List actual commands, environment and results. Identify missing prerequisites or untested behavior explicitly; do not report skipped required checks as passed.

- [ ] Relevant behavior, boundaries and consumers were checked.
- [ ] Core changes are justified; registry records are updated if the registries have been implemented.
- [ ] Coverage scope and exceptions were not weakened to hide a regression.

For schema/DSL or upstream changes, include source/target versions, migration checks on previous-release data and recovery evidence. Mark non-applicable items with a reason. Use the [transition instructions](../docs/develop/architecture-transition-ru.md) for the full procedure.
