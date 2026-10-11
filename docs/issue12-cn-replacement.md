# Issue #12: China-first format-constrained replacement

This document describes **bounded in-memory masking components**, not cryptographic
format-preserving encryption (FPE), production anonymization, identity
verification, or a complete database Apply/Validate workflow.

## API and security contract

Use \`mask.NewStrategy(contracts.StrategyRef{ID: "...", Version: "1"})\`;
all nine strategies implement the existing \`contracts.Strategy\` interface
without modifying \`APIRevision=m1a-v1\`.

- Input and output preserve \`Kind\` (\`Text\`; \`cn_date\` also supports \`Date\`).
- The HMAC key must be a \`Bytes\` value of at least 32 bytes. A nonempty
  strategy ID/version/scope/normalization version/key ID must match the plan.
- Equal typed input and full masking context produce identical output.
  A different key or scope changes the keyed derivation, **not** a guarantee
  of a different output: limited output spaces can collide.
- Context cancellation and nil/invalid input fail closed. A non-NULL source
  can never return an unchanged value. \`NULL\` remains \`NULL\`.
- \`Type.Length\` is treated as a rune-count limit; byte-bound SQL columns and
  schema constraints require additional runner-level validation.
- No original values or secret keys are written to logs, audit files,
  persistent mappings, or errors by these pure functions.

## Declared coverage (2026-10-11, synthetic-only)

| Strategy ID | Input accepted | Generated shape / constraints | Known limitations |
|---|---|---|---|
| \`cn_id_card\` | 18-character GB 11643-1999 shape; 15 explicitly listed area codes, supported birth date ≥1900, nonzero sequence, valid MOD11-2 | Selected synthetic area subset, new valid birth date, 001–999 sequence and recomputed MOD11-2 check | Area list is not nationwide or historical; no identity or non-issuance guarantee |
| \`cn_mobile\` | Bounded mainland 13x/15x/17x/18x/19x mobile prefix list, 11 ASCII digits, optional 86/+86 | Preserves country-code prefix, selects a supported prefix and eight digits | No carrier allocation, unassigned-number or contact-safety guarantee |
| \`cn_bank_card\` | 16–19 ASCII digits, nonzero first digit, valid Luhn | Preserves exact length, recomputes Luhn digit | No BIN/issuer/network claims; generated value may match an actual PAN |
| \`cn_uscc\` | 18-character supported charset, limited department/category and area lists, valid MOD31 | Supported "91" department/category, bounded area, nine-character subject and recomputed MOD31 | No registry/authenticity or unassigned-code claim |
| \`cn_postal_code\` | Six ASCII digits | Six ASCII digits | No regional, address-delivery or routing validation |
| \`cn_city\` | Seven named cities: 北京、上海、广州、深圳、杭州、成都、重庆, with explicit lowercase/titlecase pinyin and 市/ city suffix | Another city from the same bounded set; preserves Han/pinyin presentation and optional suffix | No national dictionary, generalized transliteration or historic area names |
| \`cn_person_name\` | 2–4 Han characters; bounded 19 common surnames and four compound surnames | New bounded surname plus generated Han given-name characters of the same rune count | Name validity and false positives cannot be inferred from shape; no preservation of sex/ethnicity/meaning |
| \`cn_address\` | 2–32 Han-character location/street stem plus 路/街/大道/巷, 1–4 ASCII digits and 号 | Clearly synthetic "合成区" plus sample street marker and house number | Not a comprehensive Chinese address parser; no real-address authenticity or deliverability claim |
| \`cn_date\` | Valid year-first Text (YYYY-MM-DD, YYYY/MM/DD, YYYY年M月D日) or typed Date (YYYY-MM-DD), year ≥1900 | A different valid Gregorian date, with same supported presentation | Does not retain age, day-of-week, MDY/DMY, time zone or semantic correlation |

Only allow application against authorized **disposable test databases**.
Generating a checksum-valid identifier does not ensure that it is nonexistent,
unassigned or safe to publish or send communications to. Avoid sharing actual
output identifiers externally. No transformation promises uniqueness or
privacy against linkage/inference attacks. Repeated input with one scope is
deterministic, which itself permits equality linkage.

## Tests and evidence methodology

\`internal/mask/cn_format_test.go\` defines a versioned-in-code synthetic
labelled fixture of accepted/rejected shapes for each strategy. It constructs
checksum-correct source examples in memory rather than checking in harvested
or real citizen information.

\`TestCNFormatSyntheticEvaluation\` measures **format-acceptance** TP / FP / FN
and per-rule precision/recall on the hand-authored fixtures, asserting both
that a supported source is accepted and that the generated output passes the
same structural validation. This is development-time data, **not** an
independent evaluation of PII detection, privacy, downstream database validity,
or actual anonymization. Empty classes and non-supported forms are explicitly
out of scope.

\`TestCNFormatContractAndSafety\` verifies NULL, invalid keys, type and
plan mismatches, nil/canceled contexts, destination shape and deterministic
mapping. \`TestCNFormatDateAndBoundaries\` covers leap days, typed \`Date\`,
Chinese presentation and unsupported versions. Existing
\`internal/detect/eval_test.go\` supplies the separate detection evaluation;
these two metrics must not be conflated.

Run:

\`\`\`sh
gofmt -l internal/mask
go test -count=1 -v ./internal/mask/...
go test -count=1 ./...
go vet ./...
go test -race ./internal/mask/...
\`\`\`

GitHub CI must pass on the final PR SHA; race coverage runs on Ubuntu.
MySQL Fixture smoke is not a real adapter business integration test.

## Incomplete downstream acceptance

The CLI's \`scan\`/\`mask\`/\`validate\` commands are still placeholders.
The runner, durable mapping, collision/unique/Pk-FK handling, real-database
Dry Run/Apply/strict-Validate integration and full schema/length constraints
remain with #9/#11 and associated integration tasks. Therefore #12 can only
be accepted as the **pure China-first format-replacement component**; do not
claim the complete pipeline is ready without those downstream gates.
