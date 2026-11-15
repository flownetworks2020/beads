# Guarded command writes

`bd update`, `bd assign`, `bd close` and `bd delete` accept `--if-revision` for one issue. Copy the decimal string from `bd show <id> --json` without converting it through a floating-point number. It is an opaque signed 64-bit equality token, not a sequence number.

A mismatched token writes nothing and exits 13. With `--json`, the error envelope has `code: precondition_failed`. `--force` does not bypass revision equality. Unsupported backends refuse; they do not emulate a transaction with a preliminary read.

A native claim and metadata can share the guard:

```text
bd update <id> --if-revision <revision> --claim --set-metadata reservation=<unique-id>
```

The existing native transaction sets claim ownership and status, grants a lease for durable work, and applies metadata. Wisps remain ephemeral and intentionally have no native lease. A generic status/assignee update is not a substitute for a claim.

`bd update --if-version` remains a supported legacy spelling. Do not provide both spellings. Existing status/assignee guard compatibility rules remain in force.

Actual label and outbound dependency edits invalidate the selected token in their storage transaction. Exact no-ops preserve it. Deleting or renaming a dependency target invalidates surviving sources whose outbound edges change. This token does not certify the state of all predecessor beads, and it is not an aggregate revision for every possible mutation.

After an ambiguous response, read the same issue from its owning store and reconcile its owner, reservation metadata and native lease where applicable. Do not blindly retry an old token or infer success from a stale-token refusal. A matching fresh token does not authorize taking another actor's claim.

Guarded close retains pin, template, ownership and gate checks and molecule completion on retry. It refuses `--continue`, `--suggest-next` and `--claim-next` combinations. Guarded delete retains exact-ID resolution and existing confirmation/cascade behavior.

SDK transactions that cannot publish a cross-plane target deletion and the affected source revisions atomically return unsupported before mutation. Existing pinned transactions continue to support that operation. No new storage schema or service is required.
