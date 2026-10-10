package detect

import (
	"regexp"
	"strconv"
	"strings"
)

// -------- individual matchers (pure; no network / file / DB I/O) ------

// emailPattern matches the standard user@host.tld shape; strict on both
// sides (single @, at least one dot in host, 2+ char TLD).
var emailPattern = regexp.MustCompile(`(?i)^[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}$`)

func matchEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Count(s, "@") != 1 {
		return false
	}
	return emailPattern.MatchString(s)
}

var urlPattern = regexp.MustCompile(`(?i)^https?://[^\s/$.?#].*`)

func matchURL(s string) bool {
	s = strings.TrimSpace(s)
	return urlPattern.MatchString(s)
}

func matchIPv4(s string) bool {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return false
		}
		if n < 0 || n > 255 {
			return false
		}
		if p != strconv.Itoa(n) {
			return false // forbid leading zeros like "01"
		}
	}
	return true
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`)

func matchUUID(s string) bool {
	s = strings.TrimSpace(s)
	return uuidPattern.MatchString(s)
}

// SSN uses conservative DDD-DD-DDDD formatting with area-code exclusion
// (no 000 / 666 / 900-999) and requires hyphens so that random numeric
// identifiers do not accidentally match.
func matchSSN(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 11 {
		return false
	}
	if s[3] != '-' || s[6] != '-' {
		return false
	}
	a := s[:3]
	g := s[4:6]
	l := s[7:]
	if !digits(a) || !digits(g) || !digits(l) {
		return false
	}
	if a == "000" || a == "666" {
		return false
	}
	af, _ := strconv.Atoi(a)
	if af >= 900 {
		return false
	}
	if g == "00" || l == "0000" {
		return false
	}
	return true
}

var ccStripNonDigits = strings.NewReplacer("-", "", " ", "", ".", "")

func matchCreditCard(s string) bool {
	s = strings.TrimSpace(s)
	stripped := ccStripNonDigits.Replace(s)
	hasSeparators := stripped != s
	if !digits(stripped) {
		return false
	}
	n := len(stripped)
	if n < 13 || n > 19 {
		return false
	}
	if !brandLengthOK(stripped) {
		return false
	}
	if !luhnOK(stripped) {
		return false
	}
	if n == 16 && !hasSeparators && !brandLengthOK(stripped) {
		// 16-digit plain block without brand prefix evidence is too
		// ambiguous; reject to avoid false positives.
		return false
	}
	return true
}

func brandLengthOK(s string) bool {
	n := len(s)
	switch {
	case strings.HasPrefix(s, "4"):
		return n == 13 || n == 16 || n == 19
	case strings.HasPrefix(s, "34") || strings.HasPrefix(s, "37"):
		return n == 15
	}
	if n >= 2 {
		p2, _ := strconv.Atoi(s[:2])
		if p2 >= 51 && p2 <= 55 && n == 16 {
			return true
		}
	}
	if len(s) >= 4 {
		p4, _ := strconv.Atoi(s[:4])
		if (p4 == 6011 || (p4 >= 644 && p4 <= 649) || strings.HasPrefix(s, "65")) && (n == 16 || n == 19) {
			return true
		}
	}
	return false
}

func luhnOK(s string) bool {
	sum := 0
	alt := false
	for i := len(s) - 1; i >= 0; i-- {
		n := int(s[i] - '0')
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

// zipPattern matches 5-digit ZIP or ZIP+4 with dash separator.
var zipPattern = regexp.MustCompile(`^\d{5}(-\d{4})?$`)

func matchZip(s string) bool {
	s = strings.TrimSpace(s)
	if !zipPattern.MatchString(s) {
		return false
	}
	if strings.HasPrefix(s, "00000") {
		return false
	}
	return true
}

// nanpPattern matches (123) 456-7890, 123-456-7890, 123.456.7890, 1234567890
var nanpPattern = regexp.MustCompile(`^(\+?1[\-\s.])?(\(?\d{3}\)?[\-\s.]?\d{3}[\-\s.]?\d{4})$`)

func matchPhone(s string) bool {
	s = strings.TrimSpace(s)
	if !nanpPattern.MatchString(s) {
		return false
	}
	stripped := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if len(stripped) == 11 {
		if stripped[0] != '1' {
			return false
		}
		stripped = stripped[1:]
	}
	if len(stripped) != 10 {
		return false
	}
	// NANP NXX cannot start with 0 or 1.
	if stripped[0] == '0' || stripped[0] == '1' || stripped[3] == '0' || stripped[3] == '1' {
		return false
	}
	return true
}

// ---------- address, city, full_name use synthetic tiny dictionaries.
// Any production lexicon must be explicitly approved via the Main-owned
// wording/license/source proposal per #6 scope notes.

var addressStreetSuffixes = []string{
	"ST", "AVE", "RD", "BLVD", "LN", "DR", "WAY", "CT", "PL", "HWY",
	"STE", "SUITE", "CIR", "PKWY", "TERR", "COURT",
}

func matchAddress(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	toks := strings.Fields(s)
	if len(toks) < 3 {
		return false
	}
	// Must start with a number (house number).
	if !digits(strings.TrimRight(toks[0], ",")) {
		return false
	}
	last := strings.ToUpper(strings.TrimRight(toks[len(toks)-1], ",. "))
	for _, sfx := range addressStreetSuffixes {
		if last == sfx {
			return true
		}
	}
	return false
}

// syntheticCityList is the test-city dictionary shipped with the M1
// release. No third-party city list is referenced.
var syntheticCityList = map[string]struct{}{
	"NEW YORK": {}, "LOS ANGELES": {}, "CHICAGO": {}, "HOUSTON": {},
	"PHOENIX": {}, "PHILADELPHIA": {}, "SAN ANTONIO": {}, "SAN DIEGO": {},
	"DALLAS": {}, "SAN JOSE": {}, "AUSTIN": {}, "SEATTLE": {}, "DENVER": {},
	"BOSTON": {}, "WASHINGTON": {}, "PORTLAND": {}, "ATLANTA": {}, "MIAMI": {},
	"SAN FRANCISCO": {}, "DETROIT": {}, "MINNEAPOLIS": {}, "TAMPA": {},
	"ST LOUIS": {}, "CHARLOTTE": {}, "BALTIMORE": {},
}

// usStateAbbrev is the minimal two-letter state hint dictionary used
// only to disambiguate a city-looking token from an arbitrary
// capitalized token pair.
var usStateAbbrev = map[string]struct{}{
	"AL": {}, "AK": {}, "AZ": {}, "AR": {}, "CA": {}, "CO": {}, "CT": {}, "DE": {},
	"FL": {}, "GA": {}, "HI": {}, "ID": {}, "IL": {}, "IN": {}, "IA": {}, "KS": {},
	"KY": {}, "LA": {}, "ME": {}, "MD": {}, "MA": {}, "MI": {}, "MN": {}, "MS": {},
	"MO": {}, "MT": {}, "NE": {}, "NV": {}, "NH": {}, "NJ": {}, "NM": {}, "NY": {},
	"NC": {}, "ND": {}, "OH": {}, "OK": {}, "OR": {}, "PA": {}, "RI": {}, "SC": {},
	"SD": {}, "TN": {}, "TX": {}, "UT": {}, "VT": {}, "VA": {}, "WA": {}, "WV": {},
	"WI": {}, "WY": {},
}

func matchCity(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if comma := strings.Index(s, ","); comma >= 0 {
		cityPart := strings.TrimSpace(s[:comma])
		statePart := strings.TrimSpace(s[comma+1:])
		if len(statePart) == 2 {
			statePart = strings.ToUpper(statePart)
			if _, ok := usStateAbbrev[statePart]; !ok {
				return false
			}
		}
		s = cityPart
	}
	normalized := strings.ToUpper(s)
	normalized = strings.Join(strings.Fields(normalized), " ")
	_, ok := syntheticCityList[normalized]
	return ok
}

// matchFullName accepts 2-3 capitalized space-separated tokens. Tokens
// must start with an ASCII uppercase letter followed by lowercase
// letters, hyphens, or apostrophes. This is intentionally conservative
// so that UPPERCASE column headers and ALLCAPS phrases do not match.
var nameToken = regexp.MustCompile(`^[A-Z][a-zA-Z\-']*$`)

func matchFullName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	toks := strings.Fields(s)
	if len(toks) < 2 || len(toks) > 3 {
		return false
	}
	for _, t := range toks {
		if !nameToken.MatchString(t) {
			return false
		}
	}
	return true
}

var (
	dateISO      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	dateMDYSlash = regexp.MustCompile(`^(0?[1-9]|1[0-2])/(0?[1-9]|[12]\d|3[01])/\d{4}$`)
	dateMDYDot   = regexp.MustCompile(`^(0?[1-9]|1[0-2])\.(0?[1-9]|[12]\d|3[01])\.\d{4}$`)
	dateMDYDash  = regexp.MustCompile(`^(0?[1-9]|1[0-2])-(0?[1-9]|[12]\d|3[01])-\d{4}$`)
	dateDMYSlash = regexp.MustCompile(`^(0?[1-9]|[12]\d|3[01])/(0?[1-9]|1[0-2])/\d{4}$`)
	dateDMYDash  = regexp.MustCompile(`^(0?[1-9]|[12]\d|3[01])-(0?[1-9]|1[0-2])-\d{4}$`)
)

func matchDate(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if dateISO.MatchString(s) || dateMDYSlash.MatchString(s) || dateMDYDash.MatchString(s) || dateMDYDot.MatchString(s) {
		return true
	}
	// DMY patterns: accept only when the *day* value strictly exceeds
	// 12 to disambiguate against the MDY path above (avoids double-
	// counting ambiguous inputs).
	if dateDMYSlash.MatchString(s) {
		parts := strings.Split(s, "/")
		d, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		return d > 12 && m <= 12
	}
	if dateDMYDash.MatchString(s) {
		parts := strings.Split(s, "-")
		d, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		return d > 12 && m <= 12
	}
	return false
}

// dmyStrict returns true for inputs that are unambiguously DMY dates
// (day token > 12). It is used by the caller to honor DateOrder=DMY and
// avoid inflating hit counts with ambiguous MDY/DMY patterns.
func dmyStrict(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	var dayTok, monthTok string
	if dateDMYSlash.MatchString(s) {
		p := strings.Split(s, "/")
		dayTok, monthTok = p[0], p[1]
	} else if dateDMYDash.MatchString(s) {
		p := strings.Split(s, "-")
		dayTok, monthTok = p[0], p[1]
	} else {
		return false
	}
	d, err1 := strconv.Atoi(dayTok)
	m, err2 := strconv.Atoi(monthTok)
	if err1 != nil || err2 != nil {
		return false
	}
	return d > 12 && m >= 1 && m <= 12 && d <= 31
}

// digits returns true iff s consists only of ASCII decimal digits.
func digits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
