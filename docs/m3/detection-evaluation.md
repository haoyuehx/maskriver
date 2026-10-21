# M3 detection evaluation (current evidence)

The only measured detection set is the 74-case synthetic set in
`internal/detect/eval_test.go`: 38 positives (30 in declared coverage, 8
out-of-coverage) and 36 negatives, 13 rules, `MinSamples=1`.

Measured per-rule results (2026-10-21, go1.26.3):

| Rule | TP | FP | FN | Precision | Recall | F1 |
|---|---:|---:|---:|---:|---:|---:|
| email | 2 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| url | 1 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| ip_address | 2 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| uuid | 1 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| cn_date | 2 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| cn_id_card | 2 | 0 | 1 | 1.000 | 0.667 | 0.800 |
| cn_mobile | 2 | 0 | 1 | 1.000 | 0.667 | 0.800 |
| cn_bank_card | 1 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| cn_postal_code | 2 | 0 | 0 | 1.000 | 1.000 | 1.000 |
| cn_uscc | 1 | 0 | 1 | 1.000 | 0.500 | 0.667 |
| cn_city | 7 | 0 | 2 | 1.000 | 0.778 | 0.875 |
| cn_person_name | 3 | 3 | 1 | 0.500 | 0.750 | 0.600 |
| cn_address | 4 | 0 | 2 | 1.000 | 0.667 | 0.800 |
| unclassified (abstention) | 33 | 8 | 3 | 0.805 | 0.917 | 0.857 |

Limits: this is a small, self-authored development set, not an independent
holdout or production accuracy measurement. Several rules intentionally count
out-of-coverage positives as false negatives. `cn_person_name` has material
false positives. UNKNOWN is never a safe result. M3-4's independent evaluation
set and human adjudication remain **NOT COMPLETED**; do not describe these
numbers as production precision/recall.

Synthetic strategy format evaluation remains 21 positive / 28 negative cases in
`internal/mask/cn_format_test.go`, all TP with zero FP/FN on that self-authored
set; this does not prove real-world acceptance or anonymity.
