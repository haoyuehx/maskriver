# v0.2.0-preview.1 — independent security review

- **Reviewer context:** fresh read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Reviewed candidate:** `7266a540f41e544954316ae87024108a9b2d5775`, archive
  `381430cc…c118`.
- **Verdict:** **PASS** — no P0/P1.

## Verified

Default `mask` preview is zero-write; apply requires `--approved` plus a valid
Ed25519 decision; streaming requires its own approval; recovery fails closed on
wrong key, target tamper, commit-unknown and unsupported schema; package has no
secrets/databases/`.git`; binary VCS revision and `vcs.modified=false`; NOTICE/
SBOM/licence texts match the link set; archive is deterministic and
`v0.1.0-dev` is untouched.

## P2 (disposition)

1. README claimed the release was already published -> wording changed to a
   release target with the GitHub Release page as the authority.
2. "No writes without --apply" was imprecise for explicit schema/history
   commands -> qualified as business-data writes; `init-outbox`/`init-recovery`
   are explicit schema writes.
3. Release smoke was not committed at the reviewed SHA -> committed as
   `scripts/release-smoke.py`.
4. A failed resume records terminal `state=conflict` metadata -> documented.
5. `--help` said "M1 in development" -> updated.
6. Licence texts are not machine-compared to the module cache -> accepted; the
   candidate package texts were manually verified byte-identical.
7. Untagged candidate provenance -> final artifact must be rebuilt from the
   release tag; checksums are GitHub/TLS-trusted, not signed.

## Second-context recheck

A fresh security recheck verified the busy-timeout and hot-journal changes:
read-only `status` never writes and only reports `hot_journal` after a failed
read-only open with a journal present; explicit apply/resume performs one
SQLite crash recovery, still requires `--apply` and a valid approval for any
business write; commit-unknown and transaction atomicity are unchanged; release
docs no longer claim an unpublished release. The recheck's P1 was missing
regression coverage; committed DB/CLI tests were added and pass, and the
acceptance wording was corrected.

**Recheck verdict: PASS.**
