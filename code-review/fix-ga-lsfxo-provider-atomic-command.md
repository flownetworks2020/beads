# ga-lsfxo provider repair review

The installed command adapter cannot execute the native revision-checked claim and metadata transaction. Label and outbound-edge edits also leave that revision unchanged. The repair exposes the existing transaction through four CLI verbs and invalidates affected revisions inside existing auxiliary-write transactions.

Runbook: yes — existing ga-lsfxo bead and maintained-fork/provider-pair MR trail, explicitly approved by Klas. No separate Jira record.

## Authority and architecture

The original architecture-reviewed plan was approved at 21:12 UTC on 2026-11-15. Its SHA256 is `757d8becbec468a5b3686ffc3bb523b67a833fd1dac1fc77eca131374fecaa9a`. The bounded AT5 amendment was approved at 21:25:11 UTC; SHA256 `f8918efea1d83698dafc515d45b77d74cc337cc8807c98569e644ac641fa79b9`. Both approvals are recorded on ga-lsfxo.

Architect review: the CLI transports the signed opaque token; existing storage transactions own revision checks, claims, leases and mutations. No schema, service, global writer lock or host SQL is added. Unsafe cross-plane SDK deletions with separate transactions refuse before writes. Existing pinned transactions remain supported.

## Code-review and security-review

Independent debugging-gas-issues skeptic reviewed the complete working diff. No remaining important correctness or security finding was identified. Earlier findings drove selective staging, surviving-source invalidation and fail-closed split-session deletion. The final safety regressions also cover owner protection, retry/readback, exact destructive IDs, pin/gate policy and molecule completion on retry.

Backend/java review: Java-specific style rules are not applicable to this Go repository. Go formatting, native and Windows golangci-lint, transaction/ownership/error review and independent code/security review cover this implementation. No claim is made that a Java tool or untrusted commit hook ran.

Separately identified lint maintenance: two unchanged Darwin paths had existing gosec findings. A reviewed descriptor-identity justification and corrected placement of the existing syscall-buffer annotation remove those findings without changing runtime behavior.

## Acceptance and test evidence

| Test | Result |
|---|---|
| AT1 native revision + claim + metadata | PASS through real isolated direct and proxied CLI; durable native lease asserted. Existing wisps intentionally have no lease. |
| AT2 stale metadata/authority zero-write | PASS for metadata, labels and dependencies; owner/replay safety matrix passes. |
| AT3 guarded restoration | PASS in isolated direct/proxied stores for absent and empty route, atomic cleanup and preservation of unrelated hold/custody/predecessor metadata. Live owning-store recovery remains AT8. |
| AT4 signed width, ownership, replay | Parser tests and real foreign-owner/stale/reconciled same-actor tests PASS. The lost-response scenario uses readback of a successful write; no network-fault injection is claimed. |
| AT5a–f auxiliary mutation invalidation | Real CLI, interleaved-server, durable HEAD/reopen and selective staging regressions PASS. Includes label/edge replacement, target rename/deletion and source-repo deletion; unsafe split transactions refuse. This does not claim a snapshot of predecessor state. |
| AT6 four verbs | Direct update/assign/close/delete PASS (37.050s run); proxied durable/wisp assign/close/delete and ownership/close policy PASS (50.514s run), plus earlier proxied update proof. |
| Final safety matrix | PASS 42.454s, both direct and proxied; owner/replay, exact-ID delete, boolean pin, timer gate, absent/empty route restoration. |
| Required lint | PASS: make ci-pr-lint, zero native and Windows findings. |
| Required full baseline | PASS: make test, exit 0; cmd/bd 515.746s, embeddeddolt 131.038s, total coverage 40.7%. This default baseline retains repository integration opt-ins; the explicit real-provider matrices above supply those additional proofs. |
| AT7 package/canary/installation | PENDING. No reviewed package is published or installed. |
| AT8 useful runtime outcome | PENDING. No worker recovery or broader estate acceptance is claimed. |

Fulfils-the-need: the local tests prove the previously missing provider behavior. Production fulfilment remains pending AT7–AT8 and the separate estate delivery acceptance criteria.

## Contributor provenance

The command adaptation preserves Jamel Toms's #6030 work and Julian Knutsen's #7203/#7216 interfaces and tests. Label invalidation follows Stephanie Jarmak's #5738 design. The outbound-dependency extension is this approved adaptation; the current #5738 diff does not provide it. Preserve their co-author trailers on transformed commits.

A refreshed prior-art scan also found #8285/#5793 (label timestamps) and #8026 (delete/comment race characterization). They do not supply this bounded installed-provider contract. #8285 explicitly preserves row_lock; #8026 excludes label/dependency races. No contributor PR or branch was modified, closed or superseded. The stable recovery backport must not be presented as a competing main-line replacement.

Final independent source-review binding: 51 changed/new Go files, deterministic path/content manifest SHA256 `3cf73886f136a19f975867f522f16293092c2282c35916547f4ba3828d0f42c3`. Final lint re-run after safety additions passed with zero native and Windows findings.
