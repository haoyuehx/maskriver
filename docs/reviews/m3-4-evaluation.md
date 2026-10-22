# M3-4 evaluation review

- **Head:** `3b4aeb9` (initial), fixes follow in the same PR.
- **Context:** independent QA/statistics and security/licence subagents
  (`opencode-go/deepseek-v4.1-flash`).
- **Verdict:** security/licence PASS; QA CHANGES REQUIRED on documentation and
  self-referential evidence, no production-code defect.

QA findings addressed:

- Corrected the false "fully disjoint from detector rules" claim. The holdout
  uses independently coded checksum implementations but necessarily overlaps
  bounded coverage allowlists; it measures in-coverage shape, not
  out-of-coverage recall.
- Documented the real size: 345 cases = 210 positives + 135 negatives, with
  0 false positives at this point in time.
- Masking holdout now uses test-local independent checksum/prefix validators
  instead of production helpers, asserts unique outputs, and is described as an
  invariant/regression measurement rather than anonymity evidence.
- Removed the contradictory "M3-4 NOT COMPLETED" text; human/external
  adjudication remains explicitly not completed.
- Negatives count any Sensitive classification as FP, not only the expected rule.

Accepted limits: synthetic repository-authored corpus, no human adjudication,
no real-world PII accuracy, no out-of-coverage positives, small curated
city/name/address samples. No third-party corpus, secrets or real data used.
