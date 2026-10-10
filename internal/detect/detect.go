package detect

import (
	"context"
	"math"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	defaultMinSamples = 20
	defaultMinRatio   = .9
)

// Detector implements the frozen m1a-v1 detection contract. It is immutable
// after New returns: the compiled patterns and rule table live in the value,
// so detectors share no mutable package-level state.
type Detector struct {
	rules []rule
}

var _ contracts.Detector = Detector{}

// New builds a detector whose immutable rule table covers email, URL, IP
// addresses (IPv4 and IPv6), UUID, Chinese calendar dates, and the China-first
// fields (resident identity card, mainland mobile, bank card, postal code,
// unified social credit identifier, bounded geographic names, bounded Han
// personal names, and Chinese address fragments). Context-required rules carry
// the column names that identify their field. Rules are never mutated after
// construction; add later rules by extending this table. See cn.go for the
// China-first rule-data versions and coverage limits.
func New() contracts.Detector {
	uuidPattern := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	return Detector{rules: []rule{
		{id: "email", match: emailMatch, contexts: []string{"email", "mail"}},
		{id: "url", match: urlMatch, contexts: []string{"url", "uri", "link", "website", "site", "web"}},
		{id: "ip_address", match: ipMatch, contexts: []string{"ip"}},
		{id: "uuid", match: func(v contracts.Value, _ string) bool { return uuidMatch(v, uuidPattern) }, contexts: []string{"uuid", "guid"}},
		{id: "cn_date", match: cnDateMatch, contexts: []string{"date", "birth", "birthday", "dob"}},
		{id: "cn_id_card", match: cnIDCardMatch},
		{id: "cn_mobile", match: cnMobileMatch, contexts: []string{"phone", "mobile", "tel", "telephone", "cell", "msisdn", "手机号", "手机号码", "联系电话"}, excludeContexts: []string{"order", "transaction", "订单", "交易", "流水"}, requiresContext: true},
		{id: "cn_bank_card", match: cnBankCardMatch, contexts: []string{"bank", "card", "银行卡", "卡号"}, excludeContexts: []string{"order", "transaction", "订单", "交易", "流水"}, requiresContext: true},
		{id: "cn_postal_code", match: cnPostalCodeMatch, contexts: []string{"postal", "postcode", "zip", "邮编", "邮政编码"}, excludeContexts: []string{"order", "transaction", "订单", "交易", "流水"}, requiresContext: true},
		{id: "cn_uscc", match: cnUSCCMatch, contexts: []string{"uscc", "统一社会信用代码", "社会信用代码"}},
		{id: "cn_city", match: cnCityMatch, contexts: []string{"city", "province", "region", "城市", "省", "省份", "地区", "自治区", "所在地"}, requiresContext: true},
		{id: "cn_person_name", match: cnPersonNameMatch, contexts: []string{"name", "姓名", "名字", "人名", "联系人"}, requiresContext: true},
		{id: "cn_address", match: cnAddressMatch, contexts: []string{"address", "addr", "street", "地址", "住址", "详址"}, excludeContexts: []string{"email", "mail", "ip", "url", "uri", "link", "website", "site", "web"}, requiresContext: true},
	}}
}

// Detect uses only the sample metadata and values. Unknown is deliberately
// retained for unsupported, ambiguous, or insufficient evidence; it is not a
// safe verdict. Format matches and column-context eligibility are evaluated
// separately so a phone column holding emails can be reported as conflicting.
func (d Detector) Detect(ctx context.Context, req contracts.DetectionRequest) (contracts.Decision, error) {
	if ctx == nil {
		return contracts.Decision{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return contracts.Decision{}, err
	}
	if req.MinSamples == 0 {
		req.MinSamples = defaultMinSamples
	}
	if req.MinRatio == 0 {
		req.MinRatio = defaultMinRatio
	}
	if err := validateRequest(ctx, req); err != nil {
		return contracts.Decision{}, err
	}
	base := contracts.Decision{Column: req.Column.Ref, Type: req.Column.Type, SampleBasis: req.Sample.Basis}
	if req.Override != nil {
		return forced(*req.Override), nil
	}
	if req.Reviewed != nil {
		return forced(*req.Reviewed), nil
	}
	if req.Skip {
		base.Source = contracts.Skip
		return base, nil
	}

	values, err := distinctValues(ctx, req.Sample.Values)
	if err != nil {
		return contracts.Decision{}, err
	}
	tokens := contextTokens(req.Column.Ref.Column)
	// A rule matches the values by strong format and/or has its field implied by
	// the column name. A column is classified only when exactly one rule is fully
	// eligible; the conflict rule below decides when the format and context
	// disagree with each other.
	formatHit := make(map[string]bool, len(d.rules))
	contextHit := make(map[string]bool, len(d.rules))
	rulesEligible := make(map[string]bool, len(d.rules))
	evidence := make([]contracts.Evidence, 0, len(d.rules))
	for _, r := range d.rules {
		if err := ctx.Err(); err != nil {
			return contracts.Decision{}, err
		}
		hits := 0
		for _, value := range values {
			if err := ctx.Err(); err != nil {
				return contracts.Decision{}, err
			}
			if r.match(value, req.DateOrder) {
				hits++
			}
		}
		formatStrong := len(values) >= req.MinSamples && float64(hits)/float64(len(values)) >= req.MinRatio
		contextImplied := r.matchesContext(tokens)
		ruleEligible := formatStrong && (!r.requiresContext || contextImplied)
		reasons := evidenceReasons(hits, len(values), req.MinSamples, req.MinRatio)
		if formatStrong && r.requiresContext && !contextImplied {
			// The values look like the field but the column context is absent,
			// so the format alone is not enough to call the column sensitive.
			reasons = append(reasons, contracts.InsufficientEvidence)
		}
		evidence = append(evidence, contracts.Evidence{
			RuleID:    r.id,
			Hits:      hits,
			Total:     len(values),
			Threshold: req.MinRatio,
			Eligible:  ruleEligible,
			Reasons:   reasons,
		})
		if formatStrong {
			formatHit[r.id] = true
		}
		if contextImplied {
			contextHit[r.id] = true
		}
		if ruleEligible {
			rulesEligible[r.id] = true
		}
	}
	base.Evidence = evidence
	if len(values) < req.MinSamples {
		return base, nil
	}
	if conflictingRules(rulesEligible, formatHit, contextHit) {
		for i := range evidence {
			evidence[i].Reasons = append(evidence[i].Reasons, contracts.ConflictingEvidence)
		}
		return base, nil
	}
	if len(rulesEligible) != 1 {
		return base, nil
	}
	for _, r := range d.rules {
		if rulesEligible[r.id] {
			base.Sensitivity, base.Source, base.RuleID = contracts.Sensitive, contracts.Rule, r.id
			break
		}
	}
	return base, nil
}

// conflictingRules reports a conflict when two rules fully qualify, or when a
// column-context rule that its own values do not support disagrees with another
// rule that matches the values by strong format. A context-required rule that
// matches by format only, with no context disagreement, is an unresolved weak
// candidate: it does not veto a context-free strong match such as an identity
// card number.
func conflictingRules(eligible, formatHit, contextHit map[string]bool) bool {
	if len(eligible) > 1 {
		return true
	}
	for cid := range contextHit {
		if eligible[cid] {
			// The column context is supported by the values, so it is a real
			// claim rather than a disagreement; a second claim is already
			// caught by len(eligible) > 1 above.
			continue
		}
		for id := range formatHit {
			if id != cid {
				return true
			}
		}
	}
	return false
}

// rule keeps value-format matching and column-context eligibility separate.
// contexts lists lowercase column-name tokens that identify this field, and
// excludeContexts lists contradictory tokens that cancel them. requiresContext
// marks fields too ambiguous to classify from value format alone, so they
// become Eligible only when the column context agrees.
type rule struct {
	id              string
	match           func(contracts.Value, string) bool
	contexts        []string
	excludeContexts []string
	requiresContext bool
}

func (r rule) matchesContext(tokens []string) bool {
	if r.hasExcludedContext(tokens) {
		return false
	}
	for _, keyword := range r.contexts {
		for _, token := range tokens {
			if token == keyword {
				return true
			}
			// Chinese column names usually attach the field word to a prefix
			// (用户手机号, 客户身份证号) with no separator, so a non-ASCII
			// keyword also matches as a token substring. ASCII keywords keep
			// exact-token matching so "update" never matches "date".
			if !isASCIIKeyword(keyword) && strings.Contains(token, keyword) {
				return true
			}
		}
	}
	return false
}

// hasExcludedContext reports whether the column also carries a contradictory
// token such as "order", "transaction", "订单", "交易", or "流水". A field word
// next to one of these describes a transaction or reference identifier rather
// than a person field, so the rule declines the column instead of classifying
// it.
func (r rule) hasExcludedContext(tokens []string) bool {
	for _, keyword := range r.excludeContexts {
		for _, token := range tokens {
			if token == keyword {
				return true
			}
			if !isASCIIKeyword(keyword) && strings.Contains(token, keyword) {
				return true
			}
		}
	}
	return false
}

// isASCIIKeyword reports whether every byte of s is ASCII, so callers fall back
// to exact-token matching for English column names.
func isASCIIKeyword(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func evidenceReasons(hits, total, minSamples int, minRatio float64) []contracts.IssueCode {
	var reasons []contracts.IssueCode
	if total < minSamples {
		reasons = append(reasons, contracts.InsufficientEvidence)
	}
	if total == 0 || hits == 0 {
		reasons = append(reasons, contracts.Unsupported)
	}
	if hits > 0 && total >= minSamples && float64(hits)/float64(total) < minRatio {
		reasons = append(reasons, contracts.InsufficientEvidence)
	}
	return reasons
}

// contextTokens splits a column name into lowercase word tokens on separators
// and camelCase boundaries, so "user_email" and "emailAddress" both yield the
// token "email" while ambiguous substrings such as "update" never match "date".
func contextTokens(column string) []string {
	var tokens []string
	var word strings.Builder
	var prev rune
	flush := func() {
		if word.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(word.String()))
		word.Reset()
	}
	for _, r := range column {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
				flush()
			}
			word.WriteRune(r)
		} else {
			flush()
		}
		prev = r
	}
	flush()
	return tokens
}

func text(v contracts.Value) (string, bool) {
	if v.Kind() != contracts.Text {
		return "", false
	}
	return v.Payload(), true
}

// emailMatch accepts an ASCII dot-atom address whose domain is a dotted
// hostname or a bracketed IP literal. net/mail performs structural parsing,
// then the explicit checks reject lenient parses such as "a@-host", "a@host",
// "a@exa_mple.com", or quoted/display-name forms.
func emailMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok || len(s) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	if at <= 0 || at == len(s)-1 || strings.IndexByte(s[:at], '@') >= 0 {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if len(local) > 64 || !isDotAtom(local) {
		return false
	}
	return isEmailDomain(domain)
}

const atextSpecials = "!#$%&'*+-/=?^_`{|}~"

func isDotAtom(local string) bool {
	for _, label := range strings.Split(local, ".") {
		if label == "" {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if isASCIIAlnum(c) || strings.IndexByte(atextSpecials, c) >= 0 {
				continue
			}
			return false
		}
	}
	return true
}

func isEmailDomain(domain string) bool {
	if len(domain) >= 2 && domain[0] == '[' && domain[len(domain)-1] == ']' {
		return net.ParseIP(domain[1:len(domain)-1]) != nil
	}
	return isHostname(domain)
}

func isHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !isHostLabel(label) {
			return false
		}
	}
	tld := labels[len(labels)-1]
	if len(tld) < 2 {
		return false
	}
	for i := 0; i < len(tld); i++ {
		c := tld[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

func isHostLabel(label string) bool {
	if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if isASCIIAlnum(c) || c == '-' {
			continue
		}
		return false
	}
	return true
}

func isASCIIAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// urlMatch accepts absolute http/https URLs whose host is an IP literal or a
// dotted hostname and whose port, when present, is in 1..65535. It rejects
// missing hosts ("https://?"), invalid/out-of-range ports, and empty ports.
func urlMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok || strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Opaque != "" || u.Host == "" || strings.HasSuffix(u.Host, ":") {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if net.ParseIP(host) == nil && !isHostname(host) {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

// ipMatch accepts any net.ParseIP-valid IPv4 or IPv6 literal. Zone-scoped
// forms such as "fe80::1%eth0" are not IP literals and are rejected.
func ipMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	return ok && net.ParseIP(s) != nil
}

// uuidMatch accepts RFC 4122 shape only: versions 1-5 and variants 8, 9, a, b.
// Versions 0 and 6+, the nil UUID, and non-RFC variants are rejected.
func uuidMatch(v contracts.Value, pattern *regexp.Regexp) bool {
	s, ok := text(v)
	return ok && pattern.MatchString(s)
}

// cnDateMatch accepts a valid typed Date or a calendar-valid year-first text
// date: YYYY-MM-DD, YYYY/MM/DD, or YYYY年M月D日, with year >= 1. Month/day order
// formats are deliberately unsupported; DateOrder stays a validated contract
// field only.
func cnDateMatch(v contracts.Value, _ string) bool {
	switch v.Kind() {
	case contracts.Date:
		return validDateText(v.Payload(), time.DateOnly)
	case contracts.Text:
		s := v.Payload()
		for _, layout := range [...]string{"2006-01-02", "2006/01/02", "2006年1月2日"} {
			if validDateText(s, layout) {
				return true
			}
		}
	}
	return false
}

func validDateText(s, layout string) bool {
	t, err := time.Parse(layout, s)
	return err == nil && t.Format(layout) == s && t.Year() >= 1
}

func distinctValues(ctx context.Context, values []contracts.Value) ([]contracts.Value, error) {
	seen := make(map[contracts.Value]struct{}, len(values))
	out := make([]contracts.Value, 0, len(values))
	for _, value := range values {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if value.IsNull() {
			continue
		}
		if value.Kind() == contracts.Text && strings.TrimSpace(value.Payload()) == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func validateRequest(ctx context.Context, req contracts.DetectionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c := req.Column
	if c.Ref.Database == "" || c.Ref.Schema == "" || c.Ref.Table == "" || c.Ref.Column == "" {
		return contracts.ErrInvalid
	}
	if c.Type.Kind == contracts.Null || c.Type.Kind > contracts.Instant {
		return contracts.ErrInvalid
	}
	if req.Sample.Basis != contracts.DistinctNonNull {
		return contracts.ErrInvalid
	}
	if req.DateOrder != "MDY" && req.DateOrder != "DMY" {
		return contracts.ErrInvalid
	}
	if req.MinSamples < 1 {
		return contracts.ErrInvalid
	}
	if math.IsNaN(req.MinRatio) || math.IsInf(req.MinRatio, 0) || req.MinRatio < 0 || req.MinRatio > 1 {
		return contracts.ErrInvalid
	}
	for _, value := range req.Sample.Values {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !value.IsNull() && value.Kind() != c.Type.Kind {
			return contracts.ErrInvalid
		}
	}
	if req.Override != nil && (!validDecision(*req.Override, req.Column) || req.Override.Source != contracts.Manual) {
		return contracts.ErrInvalid
	}
	if req.Reviewed != nil && (!validDecision(*req.Reviewed, req.Column) || req.Reviewed.Source != contracts.ReviewedHistory) {
		return contracts.ErrInvalid
	}
	return nil
}

func validDecision(d contracts.Decision, c contracts.Column) bool {
	return d.Column == c.Ref && d.Type == c.Type &&
		(d.Sensitivity == contracts.Sensitive || d.Sensitivity == contracts.NotSensitive) &&
		d.Source != contracts.SourceUnknown
}

// forced returns a copy of an explicit Override/Reviewed decision: it never
// echoes caller-supplied evidence that the detector did not verify.
func forced(d contracts.Decision) contracts.Decision {
	d.SampleBasis = contracts.DistinctNonNull
	d.Evidence = nil
	return d
}
