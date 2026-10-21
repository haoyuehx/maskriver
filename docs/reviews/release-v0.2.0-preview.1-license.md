# v0.2.0-preview.1 — independent licence / supply-chain review

- **Reviewer context:** fresh read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Reviewed candidate:** `7266a540f41e544954316ae87024108a9b2d5775`, archive
  `381430cc…c118`.
- **Verdict:** **PASS** — no P0/P1.

## Verified

The linux/amd64 link set is exactly the 10 external modules reported by
`go version -m` and `go list -deps`; NOTICE and SBOM agree; every NOTICE
reference resolves and every shipped licence text is referenced; all 22 texts
(including Go `LICENSE` and `PATENTS`) match their upstream/module-cache bytes;
MPL-2.0 `go-sql-driver/mysql v1.10.1` has the full text and a valid source offer;
MANIFEST/SHA256SUMS cover exactly the package; no secrets/db/`.git`; two clean
builds are byte-identical; `verify-release.sh` passes.

## P2 (disposition)

- Untagged candidate metadata -> final tag rebuild is the release step.
- Release record lacked final SHA -> final record is in the release acceptance
  comment; candidate record is in `docs/releases/v0.2.0-preview.1-acceptance.md`.
- Licence-text identity is manually verified, not machine-gated -> accepted P2.
- Licence label defaults to BSD-3-Clause for future unmapped modules ->
  `license_for` still fails closed on unmapped modules; accepted P2.
- MPL offer is pointer-only under §3.2 -> accepted.
- Custom SBOM format -> documented as `maskriver-sbom-v1`, not a standards claim.
- Attribution now includes the Go patent grant row.

## Second-context recheck

The licence/supply-chain review was performed at the reviewed candidate and
its P1 (Go toolchain licence) was fixed and rechecked independently. Final
package contents, link set, NOTICE/SBOM binding, MPL source offer, checksums and
reproducibility remain PASS. No P0/P1.

**Recheck verdict: PASS.**
