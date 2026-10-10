package detect

import (
	"strings"
	"time"
	"unicode"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file holds the China-first rules for issue #6. Every rule is a bounded,
// self-authored structure check derived from public format specifications; none
// of them is identity, registration, or entitlement verification, and none of
// the table data below is copied from a third-party dataset or source file.
// Each rule documents its owned rule-data version and its deliberately limited
// coverage.

// cnIDAreaCodes is the bounded, self-authored subset of GB/T 2260 county-level
// administrative-division codes accepted as an ID-card issuing area.
//
// Rule data version: cn-id-area-v1.
// Coverage: 15 county/district codes for a handful of provinces and
// municipalities. It is deliberately NOT the complete GB/T 2260 dataset, is not
// year-scoped, and does not claim the codes were valid for any issuance date.
// IDs from unlisted areas stay Unknown rather than being guessed.
const cnIDAreaCodes = "|110101|110105|110108|120101|310101|310115|440103|440106|" +
	"440304|330106|320102|510107|420106|610113|500106|"

// cnUSCCAuthorityCodes is the bounded, self-authored subset of GB/T 2260
// registration-authority area codes (province and city level) accepted in a
// unified social credit identifier.
//
// Rule data version: cn-uscc-area-v1. Coverage is intentionally limited and not
// the complete GB/T 2260 dataset; USCCs registered in unlisted areas stay
// Unknown.
const cnUSCCAuthorityCodes = "|110000|120000|310000|500000|440100|440300|330100|" +
	"320100|510100|420100|610100|350100|"

// cnUSCCAreaCodes additionally accepts the county-level subset because some
// county registration authorities issue codes at that level.
const cnUSCCAreaCodes = cnUSCCAuthorityCodes + cnIDAreaCodes

// cnAdminCodeAllowed reports whether code is an exact member of the delimited
// allowlist. Membership is bounded by the list constant, never by the format of
// the code itself.
func cnAdminCodeAllowed(list, code string) bool {
	return strings.Contains(list, "|"+code+"|")
}

// cnMobilePrefixes is the conservative, explicitly enumerated subset of
// mainland mobile 3-digit network prefixes accepted by cnMobileMatch.
//
// Rule data version: cn-mobile-prefix-v1.
// Coverage: 13x, 15x (150-153 and 155-159), selected 17x and 19x, and 18x. It
// is a deliberately conservative subset, NOT authoritative MCC/MNC allocation
// data and not allocation-complete: unlisted or newly allocated segments stay
// Unknown rather than being guessed.
const cnMobilePrefixes = "|130|131|132|133|134|135|136|137|138|139|" +
	"150|151|152|153|155|156|157|158|159|" +
	"170|171|173|175|176|177|178|" +
	"180|181|182|183|184|185|186|187|188|189|" +
	"191|193|195|196|198|199|"

// cnMobileMatch matches a mainland-China mobile number.
//
// Rule data version: cn-mobile-prefix-v1.
// Shape: exactly 11 ASCII digits after an optional exact "+86" or "86" country
// code; the first three digits must be one of cnMobilePrefixes. A bare leading
// "+" without "86" is rejected. The rule requires phone/mobile column context,
// so ordinary 11-digit identifiers such as order numbers are never classified
// from their format alone.
func cnMobileMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok {
		return false
	}
	switch {
	case strings.HasPrefix(s, "+86"):
		s = s[len("+86"):]
	case strings.HasPrefix(s, "86"):
		s = s[len("86"):]
	}
	if len(s) != 11 || !isASCIIDigits(s) {
		return false
	}
	return strings.Contains(cnMobilePrefixes, "|"+s[:3]+"|")
}

// cnIDCardMatch matches a resident identity card number under GB 11643-1999.
//
// Rule data version: cn-id-card-gb11643-1999-v1.
// Structure: 6-digit issuing-area code from cnIDAreaCodes + 8-digit birth date +
// 3-digit sequence (not "000") + 1 check character, all ASCII with X/x accepted.
// The birth date must be a real calendar date with year >= 1900. Coverage is
// limited to the cnIDAreaCodes subset, so this is a format check and not
// identity verification.
func cnIDCardMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok || len(s) != 18 {
		return false
	}
	if !isASCIIDigits(s[:17]) {
		return false
	}
	last := s[17]
	if last == 'x' {
		last = 'X'
	}
	if last != 'X' && !(last >= '0' && last <= '9') {
		return false
	}
	if !cnIDBirthDateValid(s[6:14]) {
		return false
	}
	if !cnAdminCodeAllowed(cnIDAreaCodes, s[:6]) {
		return false
	}
	if s[14:17] == "000" {
		return false
	}
	return last == cnIDChecksumChar(s[:17])
}

// cnIDWeightDigits encodes the 17 GB 11643-1999 MOD 11-2 position weights as
// fixed-width two-digit fields (07 09 10 05 ...); the check character maps
// 0-10 through cnIDCheckChars.
const cnIDWeightDigits = "0709100508040201060307091005080402"
const cnIDCheckChars = "10X98765432"

func cnIDChecksumChar(first17 string) byte {
	sum := 0
	for i := 0; i < 17; i++ {
		weight := int(cnIDWeightDigits[2*i]-'0')*10 + int(cnIDWeightDigits[2*i+1]-'0')
		sum += int(first17[i]-'0') * weight
	}
	return cnIDCheckChars[sum%11]
}

func cnIDBirthDateValid(ymd string) bool {
	t, err := time.Parse("20060102", ymd)
	return err == nil && t.Format("20060102") == ymd && t.Year() >= 1900
}

// cnBankCardMatch matches a 16-19 digit number that passes the Luhn checksum.
//
// Rule data version: cn-bank-card-luhn-v1.
// Scope: Luhn, length, and a nonzero first digit only. It makes no BIN, issuer,
// or bank claim, so any 16-19 digit Luhn-valid identifier can match the format;
// the rule requires bank/card column context, and order/transaction-named
// columns stay Unknown. An 18-digit value can also be a resident ID card
// number, so when both formats match the detector keeps the column Unknown
// through its conflict rule instead of guessing a field.
func cnBankCardMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok || len(s) < 16 || len(s) > 19 || !isASCIIDigits(s) {
		return false
	}
	if s[0] == '0' {
		// No card network issues a PAN with a leading zero, and reference or
		// order identifiers often start with one, so reject that shape.
		return false
	}
	return luhnValid(s)
}

// luhnValid reports whether digits is a non-empty ASCII decimal string whose
// Luhn checksum is zero. It validates its own input so a bare helper call never
// returns true for empty or non-digit data.
func luhnValid(digits string) bool {
	if digits == "" {
		return false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// cnPostalCodeMatch matches a 6-digit mainland postal-code shape.
//
// Rule data version: cn-postal-code-v1.
// Scope: exactly six ASCII digits. It makes no deliverability, routing, or
// region-allocation claim; the rule requires postal/zip column context because
// six-digit numbers are otherwise a common non-sensitive identifier.
func cnPostalCodeMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok || len(s) != 6 {
		return false
	}
	return isASCIIDigits(s)
}

// cnUSCCMatch validates a GB 32100-2015 unified social credit identifier.
//
// Rule data version: cn-uscc-gb32100-2015-v1.
// Structure: 18 characters over the 31-character alphabet (0-9 and A-H, J-N,
// P-R, T-U, W-Y; I/O/S/V/Z excluded), a supported registration department and
// organization category pair, a 6-digit administrative-area code from the USCC
// area subset, a 9-character subject code, and a weighted MOD 31 check
// character. Both the department/category pairs and the area subset are
// intentionally limited, so USCCs outside them stay Unknown; this is a format
// check and not a registration or entitlement verification.
const (
	cnUSCCCharset              = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	cnUSCCWeightDigits         = "0103092719261617202925130824103028"
	cnUSCCDepartmentCategories = "|11|51|52|53|59|91|92|93|Y1|"
)

func cnUSCCMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok || len(s) != 18 {
		return false
	}
	for i := 0; i < 18; i++ {
		if strings.IndexByte(cnUSCCCharset, s[i]) < 0 {
			return false
		}
	}
	if !strings.Contains(cnUSCCDepartmentCategories, "|"+s[:2]+"|") {
		return false
	}
	area := s[2:8]
	if !isASCIIDigits(area) || !cnAdminCodeAllowed(cnUSCCAreaCodes, area) {
		return false
	}
	return s[17] == cnUSCCChecksumChar(s[:17])
}

func cnUSCCChecksumChar(first17 string) byte {
	sum := 0
	for i := 0; i < 17; i++ {
		val := strings.IndexByte(cnUSCCCharset, first17[i])
		weight := int(cnUSCCWeightDigits[2*i]-'0')*10 + int(cnUSCCWeightDigits[2*i+1]-'0')
		sum += val * weight
	}
	return cnUSCCCharset[(31-sum%31)%31]
}

func isASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// cnCityAliasGroup pairs one Chinese canonical geographic key with its bounded
// pinyin and administrative-suffix variants, so the Chinese<->pinyin mapping is
// explicit rather than a flat word list.
type cnCityAliasGroup struct {
	canonical string
	aliases   []string
}

// cnCityAliasGroups returns the bounded, self-authored geographic alias groups
// accepted by cnCityMatch. The Chinese form is always the canonical key and the
// pinyin (and, for autonomous regions, the ethnic-qualified Chinese) forms are
// its documented variants. A fresh slice is returned on every call so the rule
// carries no mutable package-level state.
//
// Rule data version: cn-city-v1.
// Coverage: eight cities (北京/beijing, 上海/shanghai, 广州/guangzhou,
// 深圳/shenzhen, 杭州/hangzhou, 西安/xi'an, 成都/chengdu, 重庆/chongqing) and a
// limited set of provinces, municipalities, and autonomous regions (广东, 浙江,
// 江苏, 四川, 陕西, 湖北, 福建, 山东, 河南, 河北, 湖南, 安徽, 江西, 辽宁, 天津,
// 广西, 内蒙古, 新疆, 宁夏, 西藏). It is deliberately NOT a nationwide
// administrative-division dictionary and is not year-scoped; geographic names
// outside this bounded list stay Unknown rather than being guessed. There is no
// generic transliteration: only the listed pinyin aliases are recognized.
func cnCityAliasGroups() []cnCityAliasGroup {
	return []cnCityAliasGroup{
		{"北京", []string{"beijing"}},
		{"上海", []string{"shanghai"}},
		{"广州", []string{"guangzhou"}},
		{"深圳", []string{"shenzhen"}},
		{"杭州", []string{"hangzhou"}},
		{"西安", []string{"xi'an", "xian"}},
		{"成都", []string{"chengdu"}},
		{"重庆", []string{"chongqing"}},
		{"广东", []string{"guangdong"}},
		{"浙江", []string{"zhejiang"}},
		{"江苏", []string{"jiangsu"}},
		{"四川", []string{"sichuan"}},
		{"陕西", []string{"shaanxi"}},
		{"湖北", []string{"hubei"}},
		{"福建", []string{"fujian"}},
		{"山东", []string{"shandong"}},
		{"河南", []string{"henan"}},
		{"河北", []string{"hebei"}},
		{"湖南", []string{"hunan"}},
		{"安徽", []string{"anhui"}},
		{"江西", []string{"jiangxi"}},
		{"辽宁", []string{"liaoning"}},
		{"天津", []string{"tianjin"}},
		{"广西", []string{"guangxi", "广西壮族"}},
		{"内蒙古", []string{"neimenggu", "innermongolia"}},
		{"新疆", []string{"xinjiang", "新疆维吾尔"}},
		{"宁夏", []string{"ningxia", "宁夏回族"}},
		{"西藏", []string{"xizang", "tibet"}},
	}
}

// cnCityAdminSuffixes lists the trailing administrative-division words removed
// by cnCityMatch before membership is checked, so "北京市" and "Beijing City"
// both canonicalize to their allowlist entry. Delimiter boundaries keep the
// comparison an exact token match rather than a substring one.
const cnCityAdminSuffixes = "|特别行政区|自治区|自治州|地区|省|市|province|city|shi|sheng|"

// cnCityMatch matches a bounded geographic name from cnCityAliasGroups.
//
// Rule data version: cn-city-v1.
// Shape: the value is normalized by lowercasing, removing spaces, apostrophes,
// hyphens and underscores, and stripping one trailing administrative suffix,
// then compared exactly against the bounded Chinese/pinyin alias groups. The
// rule requires geographic column context (city/province/城市/省份 and similar),
// so a bare Chinese two-character word is never classified from its shape alone.
// It makes no claim that the name is current, complete, or unique, and pinyin
// outside the listed aliases is not transliterated.
func cnCityMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	return ok && cnGeographicNameAllowed(s)
}

// cnGeographicNameAllowed reports whether s is one of the bounded geographic
// aliases, optionally carrying a trailing administrative suffix.
func cnGeographicNameAllowed(s string) bool {
	name := normalizeCityName(s)
	if name == "" {
		return false
	}
	variants := cnCitySuffixVariants(name)
	for _, group := range cnCityAliasGroups() {
		keys := make([]string, 0, len(group.aliases)+1)
		keys = append(keys, group.canonical)
		keys = append(keys, group.aliases...)
		for _, key := range keys {
			normalized := normalizeCityName(key)
			for _, variant := range variants {
				if variant == normalized {
					return true
				}
			}
		}
	}
	return false
}

// cnCitySuffixVariants returns name followed by name with one trailing
// administrative suffix removed, so both "北京" and "北京市" match the canonical
// key and both "Guangzhou" and "Guangzhou City" match the pinyin alias.
func cnCitySuffixVariants(name string) []string {
	variants := []string{name}
	for _, suffix := range strings.Split(cnCityAdminSuffixes, "|") {
		if suffix == "" {
			continue
		}
		if trimmed := strings.TrimSuffix(name, suffix); trimmed != name && trimmed != "" {
			variants = append(variants, trimmed)
		}
	}
	return variants
}

// normalizeCityName lowercases and removes the separators that appear in pinyin
// and mixed forms (spaces, straight or curly apostrophes, hyphens, tabs) so
// "Xi'an", "xi an", and "Xian" normalize identically. Chinese forms pass
// through unchanged because they contain none of these separators.
func normalizeCityName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case ' ', '\t', '\n', '\r', '\'', '\u2019', '-', '_':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// cnNameInList reports whether name is an exact delimiter-separated member of
// list. It splits on the delimiter instead of using a substring search so that
// a value containing the delimiter itself cannot join two adjacent entries.
func cnNameInList(list, name string) bool {
	if name == "" {
		return false
	}
	for _, entry := range strings.Split(list, "|") {
		if entry != "" && entry == name {
			return true
		}
	}
	return false
}

// cnSingleSurnames is the bounded, self-authored allowlist of single-character
// Chinese surnames accepted by cnPersonNameMatch. A Han value that does not
// start with a listed surname stays Unknown even in a name column.
//
// Rule data version: cn-person-name-v1. Coverage is a common-surname subset,
// NOT an authoritative census list; people with rare or unlisted surnames stay
// Unknown rather than being guessed.
const cnSingleSurnames = "|王|李|张|刘|陈|杨|黄|赵|吴|周|徐|孙|马|朱|胡|郭|何|高|林|" +
	"罗|郑|梁|谢|宋|唐|许|韩|冯|邓|曹|彭|曾|田|董|袁|潘|于|蒋|蔡|余|杜|叶|程|苏|" +
	"魏|吕|丁|任|沈|姚|卢|姜|崔|钟|谭|陆|汪|范|金|石|廖|贾|夏|韦|付|方|白|邹|孟|" +
	"熊|秦|邱|江|尹|薛|闫|段|雷|侯|龙|史|陶|黎|贺|顾|毛|郝|龚|邵|万|钱|严|武|戴|" +
	"莫|孔|向|汤|成|"

// cnCompoundSurnames is the bounded, self-authored allowlist of two-character
// compound surnames accepted by cnPersonNameMatch.
//
// Rule data version: cn-person-name-v1. Compound surnames outside this subset
// stay Unknown, and a compound surname is only accepted when a given name
// follows it (three or four Han characters total).
const cnCompoundSurnames = "|欧阳|司马|上官|诸葛|令狐|慕容|皇甫|尉迟|长孙|宇文|"

// cnPersonNameMatch matches a bounded Han personal-name shape.
//
// Rule data version: cn-person-name-v1.
// Shape: two to four Han characters whose leading one (or, for a listed
// compound surname, two) characters are a known surname. It is deliberately NOT
// "any two to four Han characters": an unlisted leading character such as 普
// declines, so ordinary short Chinese words stay Unknown. A value that is a
// bounded geographic alias (cnGeographicNameAllowed) also declines, because a
// place name is a stronger competing claim than a surname+shape guess and would
// otherwise produce a known false positive such as 成都 under a name column,
// which stays Unknown. The rule requires name column context, and it still does
// not exclude every ordinary word that starts with a listed surname
// (李子/白天/石头 are documented limitations, see the synthetic evaluation).
func cnPersonNameMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok {
		return false
	}
	if cnGeographicNameAllowed(s) {
		return false
	}
	runes := []rune(s)
	if len(runes) < 2 || len(runes) > 4 {
		return false
	}
	for _, r := range runes {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	if len(runes) >= 3 && cnNameInList(cnCompoundSurnames, string(runes[:2])) {
		return true
	}
	return cnNameInList(cnSingleSurnames, string(runes[:1]))
}

// cnAddressAdminWords, cnAddressRoadWords, cnAddressBuildingWords, and
// cnAddressUnitWords are the bounded, self-authored marker lists consumed by the
// ordered scanner in cnAddressStructured. cnAddressNumeralRunes lists the ASCII
// and Chinese numeral runes that may start a house/room component.
//
// Rule data version: cn-address-v1. The words are structural markers, not an
// administrative-division or street dictionary: no place name is enumerated and
// nothing is downloaded.
const cnAddressAdminWords = "|特别行政区|自治区|自治州|地区|省|市|区|县|镇|乡|街道|"
const cnAddressRoadWords = "|大道|大街|路|街|巷|弄|胡同|"
const cnAddressBuildingWords = "|大厦|公寓|小区|广场|"
const cnAddressUnitWords = "|单元|号|栋|幢|座|楼|室|房|层|院|"
const cnAddressNumeralRunes = "零一二三四五六七八九十百千万两〇"

// cnAddress marker kinds returned by cnAddressMarkerPrefix.
const (
	cnAddressKindNone = iota
	cnAddressKindAdmin
	cnAddressKindRoad
	cnAddressKindBuilding
	cnAddressKindUnit
)

// cnAddressMatch matches an ordered Chinese address fragment.
//
// Rule data version: cn-address-v1.
// Shape: 4 to 120 runes with no control characters, decomposed left to right
// into non-overlapping components. Han place-name stems, administrative markers,
// road/street and building markers (all of which must follow a non-empty Han
// place-name stem), and house/room components of a numeral (ASCII or Chinese)
// immediately followed by a unit marker are consumed in order. The value must
// contribute at least one place marker and at least one house/room component,
// and once a house component has been seen no further unrecognized Han text may
// follow. Non-Han runes other than ASCII digits and a leading house number are
// rejected, and a standalone unit marker before any numbered house is rejected.
// Ordinary notices and descriptive sentences are therefore rejected rather than
// matched by keyword overlap: "城市道路管理" has no house component, "街道通知"
// has no house component, "社区1号公告" appends prose after its house component,
// and "区县1号" has no Han place stem before its administrative markers; all stay
// Unknown. Partial fragments such as "北京市" and room-only fragments such as
// "3栋2单元501室" also stay Unknown because the evidence is insufficient.
// English street addresses are out of coverage. The rule requires address column
// context.
func cnAddressMatch(v contracts.Value, _ string) bool {
	s, ok := text(v)
	if !ok {
		return false
	}
	runes := []rune(s)
	if len(runes) < 4 || len(runes) > 120 {
		return false
	}
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return false
	}
	return cnAddressStructured(runes)
}

// cnAddressStructured scans runes as an ordered address and reports whether it
// contains at least one place marker and one house/room component with no
// trailing prose. Place-name stems must be non-marker Han runes, and every
// administrative, road, or building marker needs such a stem immediately before
// it, so bare "区县1号", foreign "ABC路1号", or punctuation "!!路1号" never
// classify. A standalone unit marker before any numbered house is rejected.
func cnAddressStructured(runes []rune) bool {
	hasPlace := false
	hasHouse := false
	seenHouse := false
	stemSinceMarker := 0
	for i := 0; i < len(runes); {
		if n := cnAddressNumberPrefix(runes, i); n > 0 {
			if unit := cnAddressWordPrefix(runes, i+n, cnAddressUnitWords); unit > 0 {
				hasHouse = true
				seenHouse = true
				stemSinceMarker = 0
				i += n + unit
				continue
			}
			// A numeral not followed by a unit marker is part of a place stem
			// such as 三里屯 or 二七, and is consumed as a Han stem rune below.
		}
		length, kind := cnAddressMarkerPrefix(runes, i)
		switch kind {
		case cnAddressKindAdmin, cnAddressKindRoad, cnAddressKindBuilding:
			if stemSinceMarker == 0 {
				return false
			}
			hasPlace = true
			i += length
			stemSinceMarker = 0
		case cnAddressKindUnit:
			if !seenHouse {
				return false
			}
			i += length
			stemSinceMarker = 0
		default:
			if seenHouse || !unicode.Is(unicode.Han, runes[i]) {
				return false
			}
			stemSinceMarker++
			i++
		}
	}
	return hasPlace && hasHouse
}

// cnAddressMarkerPrefix returns the length and kind of the longest address
// marker at runes[i], or (0, cnAddressKindNone) when none matches.
func cnAddressMarkerPrefix(runes []rune, i int) (length, kind int) {
	length, kind = cnAddressWordPrefix(runes, i, cnAddressAdminWords), cnAddressKindAdmin
	for _, candidate := range []struct {
		words string
		kind  int
	}{
		{cnAddressRoadWords, cnAddressKindRoad},
		{cnAddressBuildingWords, cnAddressKindBuilding},
		{cnAddressUnitWords, cnAddressKindUnit},
	} {
		if n := cnAddressWordPrefix(runes, i, candidate.words); n > length {
			length, kind = n, candidate.kind
		}
	}
	if length == 0 {
		return 0, cnAddressKindNone
	}
	return length, kind
}

// cnAddressWordPrefix returns the rune length of the longest delimiter-separated
// word in words that is a prefix of runes[i:], or zero.
func cnAddressWordPrefix(runes []rune, i int, words string) int {
	best := 0
	for _, word := range strings.Split(words, "|") {
		if word == "" {
			continue
		}
		wordRunes := []rune(word)
		if i+len(wordRunes) > len(runes) {
			continue
		}
		if string(runes[i:i+len(wordRunes)]) == word && len(wordRunes) > best {
			best = len(wordRunes)
		}
	}
	return best
}

// cnAddressNumberPrefix returns the rune length of the ASCII or Chinese numeral
// run starting at runes[i], or zero.
func cnAddressNumberPrefix(runes []rune, i int) int {
	n := 0
	for i+n < len(runes) {
		r := runes[i+n]
		if (r >= '0' && r <= '9') || strings.ContainsRune(cnAddressNumeralRunes, r) {
			n++
			continue
		}
		break
	}
	return n
}
