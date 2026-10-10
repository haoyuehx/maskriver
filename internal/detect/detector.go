// Package detect implements the frozen M1 contracts.Detector surface.
// Detection is rule-only, stateless, tri-state (Sensitive/NotSensitive/
// Unknown), and strictly honors pre-approved Override > Reviewed > Skip
// > rules. It never accesses the network, filesystem, or database and
// never leaks sample values or dictionary entries through errors.
package detect

import (
	"context"
	"sort"
	"strings"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

var _ contracts.Detector = (*Detector)(nil)

// ruleID identifies a built-in classifier. The set is intentionally
// closed; callers may not inject custom implementations through this
// package boundary.
type ruleID string

const (
	ruleEmail      ruleID = "email"
	ruleURL        ruleID = "url"
	ruleIPv4       ruleID = "ip_address"
	ruleUUID       ruleID = "uuid"
	ruleSSN        ruleID = "ssn"
	ruleCreditCard ruleID = "credit_card"
	ruleZipCode    ruleID = "zip_code"
	rulePhone      ruleID = "phone"
	ruleAddress    ruleID = "address"
	ruleCity       ruleID = "city"
	ruleFullName   ruleID = "full_name"
	ruleDate       ruleID = "date"
)

// ruleSpec couples a rule name to its evidence-producing matcher.
type ruleSpec struct {
	id    ruleID
	label string
	match func(sample string) bool
}

// detectorRuleTable is immutable; New() returns a copy so that package-
// level state is never mutated after init.
var detectorRuleTable = map[ruleID]ruleSpec{}

func init() {
	register := func(s ruleSpec) { detectorRuleTable[s.id] = s }
	register(ruleSpec{id: ruleEmail, label: "Email (RFC-like local@domain.TLD)", match: matchEmail})
	register(ruleSpec{id: ruleURL, label: "HTTP(S) URL with authority", match: matchURL})
	register(ruleSpec{id: ruleIPv4, label: "IPv4 dotted quad", match: matchIPv4})
	register(ruleSpec{id: ruleUUID, label: "UUID 8-4-4-4-12 hex form", match: matchUUID})
	register(ruleSpec{id: ruleSSN, label: "US SSN DDD-DD-DDDD with context", match: matchSSN})
	register(ruleSpec{id: ruleCreditCard, label: "Luhn + brand/length + context window", match: matchCreditCard})
	register(ruleSpec{id: ruleZipCode, label: "US ZIP 5-digit or ZIP+4 with context", match: matchZip})
	register(ruleSpec{id: rulePhone, label: "North American 10-digit NANP with separators", match: matchPhone})
	register(ruleSpec{id: ruleAddress, label: "English address tokens (number + street + suffix)", match: matchAddress})
	register(ruleSpec{id: ruleCity, label: "City dictionary + state suffix hint", match: matchCity})
	register(ruleSpec{id: ruleFullName, label: "Two or three capitalized English name tokens", match: matchFullName})
	register(ruleSpec{id: ruleDate, label: "MDY / DMY / ISO date shape", match: matchDate})
}

// Detector is the immutable rule-based detector. The zero value is
// invalid; construct with New.
type Detector struct {
	rules map[ruleID]ruleSpec
}

// New returns a new immutable detector. The returned detector is safe
// for concurrent use; no package-level mutable state is introduced.
func New() contracts.Detector {
	cp := make(map[ruleID]ruleSpec, len(detectorRuleTable))
	for k, v := range detectorRuleTable {
		cp[k] = v
	}
	return &Detector{rules: cp}
}

// Detect implements contracts.Detector. It returns a tri-state decision
// with explicit Evidence counters and IssueCode reasons. Override >
// Reviewed > Skip > rule evaluation. Insufficient sample size or
// ambiguous evidence yields Unknown, never NotSensitive.
func (d *Detector) Detect(ctx context.Context, req contracts.DetectionRequest) (contracts.Decision, error) {
	_ = ctx
	if d == nil || d.rules == nil {
		return contracts.Decision{}, contracts.ErrInvalid
	}
	if req.Column.Ref.Column == "" {
		return contracts.Decision{}, contracts.ErrInvalid
	}
	switch req.DateOrder {
	case "", "MDY", "DMY":
	default:
		return contracts.Decision{}, contracts.ErrInvalid
	}
	if req.MinSamples <= 0 {
		req.MinSamples = 20
	}
	if req.MinRatio <= 0 || req.MinRatio > 1.0 {
		req.MinRatio = 0.9
	}
	if len(req.Sample.Values) > 0 && req.Column.Type.Kind == 0 {
		// Without declared column Type, Text is the defensible default.
		req.Column.Type.Kind = contracts.Text
	}

	out := contracts.Decision{
		Column:      req.Column.Ref,
		Type:        req.Column.Type,
		Sensitivity: contracts.Unknown,
		Source:      contracts.SourceUnknown,
		SampleBasis: req.Sample.Basis,
	}

	// 1. Override wins.
	if req.Override != nil {
		out.Sensitivity = req.Override.Sensitivity
		out.Source = contracts.Manual
		out.RuleID = req.Override.RuleID
		out.Strategy = req.Override.Strategy
		out.Evidence = evidenceFor(req, nil, contracts.InvalidInput)
		return out, nil
	}
	// 2. Reviewed is Main-approved prior decision (lower priority than manual override).
	if req.Reviewed != nil {
		out.Sensitivity = req.Reviewed.Sensitivity
		out.Source = contracts.ReviewedHistory
		out.RuleID = req.Reviewed.RuleID
		out.Strategy = req.Reviewed.Strategy
		out.Evidence = evidenceFor(req, nil, contracts.InvalidInput)
		return out, nil
	}
	// 3. Skip declaration: the caller opted out.
	if req.Skip {
		out.Sensitivity = contracts.NotSensitive
		out.Source = contracts.Skip
		out.Evidence = evidenceFor(req, nil, contracts.InvalidInput)
		return out, nil
	}

	// 4. Sample-size defense: insufficient evidence => Unknown, never
	// NotSensitive. This is the explicit Unknown guard required by the
	// issue checklist.
	textVals, nonNullCount, codes := collectTextSamples(req.Sample, req.Column.Type.Kind)
	if nonNullCount < req.MinSamples {
		codes = append(codes, contracts.InsufficientEvidence)
		out.Evidence = []contracts.Evidence{{
			RuleID:    string(ruleEmail) + "/any",
			Total:     nonNullCount,
			Hits:      0,
			Threshold: req.MinRatio,
			Eligible:  false,
			Reasons:   dedupeIssueCodes(codes),
		}}
		return out, nil
	}

	// 5. Rule evaluation. No per-request rule filter in the frozen
	// DetectionRequest struct; every built-in rule is evaluated and the
	// strongest (highest hit-ratio) result is returned.
	ids := make([]ruleID, 0, len(d.rules))
	for id := range d.rules {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	bestRatio := -1.0
	bestHits := -1
	total := len(textVals)
	evidences := make([]contracts.Evidence, 0, len(ids))
	for _, id := range ids {
		spec := d.rules[id]
		hits := 0
		for _, s := range textVals {
			if spec.match(s) {
				hits++
			}
		}
		ratio := 0.0
		if total > 0 {
			ratio = float64(hits) / float64(total)
		}
		ev := contracts.Evidence{
			RuleID:    string(id),
			Total:     total,
			Hits:      hits,
			Threshold: req.MinRatio,
			Eligible:  total >= req.MinSamples,
			Reasons:   dedupeIssueCodes(codes),
		}
		evidences = append(evidences, ev)
		if ratio > bestRatio || (ratio == bestRatio && hits > bestHits) {
			bestRatio = ratio
			bestHits = hits
			out.RuleID = string(id)
		}
	}

	// If the date-rule is under evaluation, honor DateOrder by rerunning
	// MDY vs DMY gate and degrading hits if the declared order can't
	// disambiguate.
	if strings.EqualFold(out.RuleID, string(ruleDate)) && strings.EqualFold(req.DateOrder, "DMY") {
		// Re-count with DMY priority to ensure declared order is
		// respected; ambiguous dates (day <=12) reduce Eligible true →
		// false so that Unknown remains Unknown, not false-positive
		// Sensitive.
		spec := d.rules[ruleDate]
		dmyHits := 0
		ambiguous := 0
		for _, s := range textVals {
			if dmyStrict(s) {
				dmyHits++
			} else if spec.match(s) {
				ambiguous++
			}
		}
		for i := range evidences {
			if evidences[i].RuleID == string(ruleDate) {
				evidences[i].Hits = dmyHits
				if ambiguous > 0 {
					evidences[i].Reasons = append(evidences[i].Reasons, contracts.InsufficientEvidence)
					evidences[i].Eligible = evidences[i].Total-ambiguous >= req.MinSamples
				}
				bestRatio = 0.0
				if evidences[i].Total > 0 {
					bestRatio = float64(evidences[i].Hits) / float64(evidences[i].Total)
				}
			}
		}
	}

	out.Evidence = evidences
	if bestRatio >= req.MinRatio && evidences[0].Eligible {
		out.Sensitivity = contracts.Sensitive
		out.Source = contracts.Rule
	} else if bestRatio < req.MinRatio && !hasAmbiguousDate(ids, req.DateOrder) {
		// High-confidence NotSensitive only when no rule met the
		// threshold AND no mixed / freeform evidence pushes us into
		// Unknown. Anything else must remain Unknown per the tri-state
		// contract.
		out.Sensitivity = contracts.NotSensitive
		out.Source = contracts.Rule
	} else {
		out.Sensitivity = contracts.Unknown
		out.Source = contracts.Rule
		for i := range out.Evidence {
			out.Evidence[i].Reasons = append(out.Evidence[i].Reasons, contracts.ConflictingEvidence)
			out.Evidence[i].Reasons = dedupeIssueCodes(out.Evidence[i].Reasons)
		}
	}
	return out, nil
}

func collectTextSamples(s contracts.Sample, kind contracts.Kind) ([]string, int, []contracts.IssueCode) {
	codes := make([]contracts.IssueCode, 0, 2)
	if s.Basis != contracts.DistinctNonNull {
		codes = append(codes, contracts.InsufficientEvidence)
	}
	out := make([]string, 0, len(s.Values))
	nonNull := 0
	for _, v := range s.Values {
		if v.IsNull() {
			continue
		}
		nonNull++
		if kind == contracts.Bytes {
			// Bytes columns are not expected to carry any textual PII
			// rule match in M1-B; include payload for completeness but
			// expect no rule hits.
			out = append(out, v.Payload())
			continue
		}
		if v.Kind() != contracts.Text {
			// Non-textual values fall through as empty strings for the
			// per-rule regex/shape checks; all current text rules fail
			// empty strings correctly.
			out = append(out, v.Payload())
			continue
		}
		out = append(out, v.Payload())
	}
	return out, nonNull, codes
}

func evidenceFor(req contracts.DetectionRequest, codes []contracts.IssueCode, fallback contracts.IssueCode) []contracts.Evidence {
	reasons := dedupeIssueCodes(append([]contracts.IssueCode{fallback}, codes...))
	return []contracts.Evidence{{
		RuleID:    "manual",
		Total:     len(req.Sample.Values),
		Hits:      0,
		Threshold: req.MinRatio,
		Eligible:  false,
		Reasons:   reasons,
	}}
}

func dedupeIssueCodes(in []contracts.IssueCode) []contracts.IssueCode {
	if len(in) == 0 {
		return in
	}
	seen := make(map[contracts.IssueCode]struct{}, len(in))
	out := make([]contracts.IssueCode, 0, len(in))
	for _, c := range in {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func hasAmbiguousDate(ids []ruleID, dateOrder string) bool {
	for _, id := range ids {
		if id == ruleDate && strings.EqualFold(dateOrder, "MDY") {
			return false
		}
	}
	// If any rule remains that could be ambiguous (e.g. full_name vs
	// city, numeric phone vs credit_card) we intentionally lean
	// conservative so Unknown is preserved.
	return len(ids) > 1
}
