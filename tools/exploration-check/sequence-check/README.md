# Synthetic exploration sequence checks

Run from the repository root with the bundled Windows Go toolchain:

```powershell
go run .\tools\exploration-check\sequence-check
```

The CLI emits one deterministic JSON report to stdout. Redirect it to the Q3 evidence directory when saving a run:

```powershell
go run .\tools\exploration-check\sequence-check > .\work\luna6-exploration\q3\report.json
```

The report runs three named cases through `internal/exploration`: a segmented journey with an explicit zero-time edge and a mid-route detour; two same-ID instance snapshots with independent world/branch bindings; and an inside/outside dungeon sequence compared across JSON snapshot round trips. It records sample hashes, definition hashes, actual revisions, increments, diffs, projected views, and expected rejection errors. The cases use synthetic inputs only and are marked not installable.

JSON representation round trips are not save restoration. Independent snapshots and permit bindings are local verification fixtures; they do not create a Story branch, persist game state, or prove runtime integration. Journey calculation does not claim to prevent repeated time charging. `source_ref` values are declared by the synthetic cases and are not checked against formal game facts.

The command accepts no parameters. Unexpected arguments exit with code 2; failed scenario checks exit with code 1; a fully passing report exits with code 0. No report files are written by the CLI itself.
