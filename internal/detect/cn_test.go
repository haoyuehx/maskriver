package detect

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file tests the China-first rules in cn.go with synthetic values only.
// Check digits are produced by local generators and cross-checked against a few
// hand-computed vectors, which reduces but does not eliminate the chance of a
// shared defect. Failure messages deliberately never print sample payloads.

// testIDChecksum recomputes the GB 11643-1999 check character independently of
// cnIDChecksumChar.
func testIDChecksum(first17 string) byte {
	const table = "10X98765432"
	weights := [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	if len(first17) != 17 {
		panic("first17 must be 17 digits")
	}
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(first17[i]-'0') * weights[i]
	}
	return table[sum%11]
}

func genTestID(area, birth string, seq int) string {
	base := fmt.Sprintf("%s%s%03d", area, birth, seq)
	return base + string(testIDChecksum(base))
}

// testUSCCChecksum recomputes the GB 32100-2015 check character independently
// of cnUSCCChecksumChar.
func testUSCCChecksum(first17 string) byte {
	const charset = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	weights := [17]int{1, 3, 9, 27, 19, 26, 16, 17, 20, 29, 25, 13, 8, 24, 10, 30, 28}
	if len(first17) != 17 {
		panic("first17 must be 17 characters")
	}
	sum := 0
	for i := 0; i < 17; i++ {
		sum += strings.IndexByte(charset, first17[i]) * weights[i]
	}
	return charset[(31-sum%31)%31]
}

func genTestUSCC(departmentCategory, area, organization string) string {
	base := departmentCategory + area + organization
	return base + string(testUSCCChecksum(base))
}

// testLuhn is an independent Luhn implementation used to synthesize and check
// bank-card-shaped values.
func testLuhn(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		n := int(digits[i] - '0')
		if double {
			if n *= 2; n > 9 {
				n -= 9
			}
		}
		sum += n
		double = !double
	}
	return sum%10 == 0
}

func genTestBankCard(prefix string, length int) string {
	body := prefix
	for len(body) < length-1 {
		body += "0"
	}
	body = body[:length-1]
	for d := 0; d <= 9; d++ {
		candidate := body + strconv.Itoa(d)
		if testLuhn(candidate) {
			return candidate
		}
	}
	panic("no Luhn check digit found")
}

func cnDetect(t *testing.T, column string, minSamples int, values ...string) contracts.Decision {
	t.Helper()
	req := testRequest(textValues(t, values...))
	req.Column.Ref.Column = column
	if minSamples > 0 {
		req.MinSamples = minSamples
	}
	return detectOK(t, req)
}

func TestCnChecksumHelperVectors(t *testing.T) {
	// Hand-computed MOD 11-2 remainders, including remainder 2 -> X.
	if cnIDChecksumChar("11010119900101001") != '5' {
		t.Fatal("cnIDChecksumChar did not return the hand-computed MOD 11-2 character")
	}
	if cnIDChecksumChar("11010119900101004") != 'X' {
		t.Fatal("cnIDChecksumChar did not map remainder 2 to X")
	}
	// Hand-computed MOD 31 characters for two different structures.
	if cnUSCCChecksumChar("9144010000000000A") != '3' {
		t.Fatal("cnUSCCChecksumChar did not return the hand-computed department 9 character")
	}
	if cnUSCCChecksumChar("51110000123456789") != 'W' {
		t.Fatal("cnUSCCChecksumChar did not return the hand-computed department 5 character")
	}
}

func TestLuhnHelperBoundaries(t *testing.T) {
	for i, value := range []string{"79927398713", "4111111111111111", "0"} {
		if !luhnValid(value) {
			t.Fatalf("valid Luhn vector %d rejected by helper", i)
		}
	}
	for i, value := range []string{"", "79927398710", "12a4", "abc", " 123"} {
		if luhnValid(value) {
			t.Fatalf("invalid Luhn vector %d accepted by helper", i)
		}
	}
}

func TestCnIDCardKnownVectorAndGeneratorAgree(t *testing.T) {
	// Hand-computed from GB 11643-1999 MOD 11-2: area 110101, birth 19900101,
	// sequence 001 -> check character 5.
	const known = "110101199001010015"
	if got := genTestID("110101", "19900101", 1); got != known {
		t.Fatal("ID checksum generator disagrees with the hand-computed known vector")
	}
	r := cnDetect(t, "c", 1, known)
	if r.Sensitivity != contracts.Sensitive || r.Source != contracts.Rule || r.RuleID != "cn_id_card" {
		t.Fatal("known ID vector not detected as cn_id_card")
	}
}

func TestCnIDCardPositives(t *testing.T) {
	cases := []string{
		genTestID("110101", "19900101", 1),
		genTestID("110108", "20000229", 42), // leap year
		genTestID("440304", "19851231", 999),
		genTestID("610113", "20150601", 7),
		"11010119900101004X", // hand-computed X check character
		"11010119900101004x", // lowercase x accepted
	}
	for i, value := range cases {
		r := cnDetect(t, "c", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_id_card" {
			t.Fatalf("ID positive case %d rejected", i)
		}
	}
}

func TestCnIDCardNegatives(t *testing.T) {
	cases := map[string]string{
		"checksum":       "110101199001010016",
		"invalid_month":  genTestID("110101", "19901301", 1),
		"invalid_day":    genTestID("110101", "19900230", 1),
		"leap_not_leap":  genTestID("110101", "20010229", 1),
		"unlisted_area":  genTestID("999999", "19900101", 1),
		"zero_sequence":  genTestID("110101", "19900101", 0),
		"too_short":      "11010119900101001",
		"too_long":       "1101011990010100155",
		"letter_body":    "1101011990010100A5",
		"bad_check_char": "11010119900101001A",
		"all_letters":    "abcdefghijklmnopqr",
		"pre_1900_year":  genTestID("110101", "18991231", 1),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			r := cnDetect(t, "c", 1, value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("ID negative case %q accepted", name)
			}
			ev := evidenceByRule(t, r, "cn_id_card")
			if ev.Hits != 0 || ev.Eligible {
				t.Fatalf("ID negative case %q produced a match", name)
			}
		})
	}
}

func TestCnMobileBoundedPrefixRule(t *testing.T) {
	for i, value := range []string{
		"13800138000",
		"+8613800138000",
		"8613800138000",
		"15012345678",
		"19912345678",
	} {
		r := cnDetect(t, "phone", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_mobile" {
			t.Fatalf("mobile positive case %d rejected", i)
		}
	}

	for i, value := range []string{
		"12800138000",   // second digit 2
		"23800138000",   // wrong first digit
		"1380013800",    // 10 digits
		"138001380000",  // 12 digits
		"138-0013-8000", // separators
		"13800138000x",  // trailing letter
		"861380013800",  // country code but too short
		"",
	} {
		r := cnDetect(t, "phone", 1, value)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("mobile negative case %d accepted", i)
		}
	}
}

func TestCnMobileCountryCodeParsing(t *testing.T) {
	for i, value := range []string{"13800138000", "8613800138000", "+8613800138000"} {
		r := cnDetect(t, "phone", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_mobile" {
			t.Fatalf("well-formed country-code form %d rejected", i)
		}
	}
	for i, value := range []string{
		"+13800138000", // bare plus, no 86 country code
		"+86",
		"86",
		"861380013800",
		"86+13800138000",
		"+8613800138000extra",
	} {
		r := cnDetect(t, "phone", 1, value)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("malformed country-code form %d classified", i)
		}
	}
}

func TestCnMobileUnsupportedPrefixesUnknown(t *testing.T) {
	for i, value := range []string{
		"14012345678", // 14x not in the conservative subset
		"15412345678", // gap inside 15x
		"16012345678", // 16x not in the conservative subset
		"17212345678", // gap inside 17x
		"17912345678", // gap inside 17x
		"19012345678", // 19x not selected
		"19212345678", // 19x not selected
		"19712345678", // 19x not selected
		"12345678901", // 12x
	} {
		r := cnDetect(t, "phone", 1, value)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("unsupported mobile prefix case %d classified", i)
		}
	}
	for i, value := range []string{"13000000000", "13900000000", "19900000000"} {
		r := cnDetect(t, "phone", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_mobile" {
			t.Fatalf("supported boundary prefix case %d rejected", i)
		}
	}
}

func TestCnMobileRequiresContext(t *testing.T) {
	values := cnMobiles(20)
	t.Run("generic column stays Unknown", func(t *testing.T) {
		r := cnDetect(t, "c", 0, values...)
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("generic column classified")
		}
		ev := evidenceByRule(t, r, "cn_mobile")
		if !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
			t.Fatalf("cn_mobile evidence reasons missing insufficient evidence")
		}
	})
	for i, column := range []string{"phone", "mobile", "手机号", "联系电话", "用户手机号"} {
		t.Run("context "+column, func(t *testing.T) {
			r := cnDetect(t, column, 0, values...)
			if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_mobile" {
				t.Fatalf("context case %d not classified", i)
			}
		})
	}
	t.Run("order_number column stays Unknown", func(t *testing.T) {
		r := cnDetect(t, "order_number", 0, values...)
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("order_number column classified")
		}
	})
}

func TestCnBankCardLuhnAndContext(t *testing.T) {
	cards := make([]string, 0, 4)
	for _, length := range []int{16, 17, 18, 19} {
		card := genTestBankCard("6200000000000", length)
		if len(card) != length || !testLuhn(card) {
			t.Fatal("generated card fixture has the wrong shape")
		}
		cards = append(cards, card)
	}
	for i, card := range cards {
		r := cnDetect(t, "bank_card", 1, card)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_bank_card" {
			t.Fatalf("bank card positive case %d rejected", i)
		}
	}

	t.Run("requires bank context", func(t *testing.T) {
		r := cnDetect(t, "c", 1, cards[0])
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("bank card classified without context")
		}
	})
	for i, column := range []string{"order_number", "transaction_id"} {
		t.Run("non-bank column "+column, func(t *testing.T) {
			r := cnDetect(t, column, 1, cards[0])
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("non-bank column case %d classified", i)
			}
		})
	}

	t.Run("invalid luhn", func(t *testing.T) {
		card := cards[0]
		last := card[len(card)-1]
		alt := byte('0' + (last-'0'+1)%10)
		bad := card[:len(card)-1] + string(alt)
		if testLuhn(bad) {
			t.Fatal("mutated card fixture still passes Luhn")
		}
		r := cnDetect(t, "bank_card", 1, bad)
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("non-Luhn value classified")
		}
	})
	t.Run("out of range length", func(t *testing.T) {
		for i, length := range []int{15, 20} {
			r := cnDetect(t, "bank_card", 1, strings.Repeat("6", length))
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("out-of-range length case %d classified", i)
			}
		}
	})
}

func TestCnBankCardShapeBoundaries(t *testing.T) {
	// Leading-zero and all-zero fixtures satisfy Luhn but are not card shapes.
	for i, value := range []string{"0000000000000000", strings.Repeat("0", 19)} {
		if !luhnValid(value) {
			t.Fatalf("leading-zero fixture %d does not satisfy Luhn", i)
		}
		r := cnDetect(t, "bank_card", 1, value)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("leading-zero PAN shape %d classified", i)
		}
	}
	card := genTestBankCard("6200000000000", 16)
	r := cnDetect(t, "bank_card", 1, card)
	if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_bank_card" {
		t.Fatal("valid non-leading-zero PAN not classified with bank context")
	}
}

func TestCnPostalCodeContext(t *testing.T) {
	for i, value := range []string{"100000", "201100", "518000"} {
		r := cnDetect(t, "postal_code", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_postal_code" {
			t.Fatalf("postal positive case %d rejected", i)
		}
	}
	for i, column := range []string{"邮编", "邮政编码", "zip", "客户邮编"} {
		r := cnDetect(t, column, 1, "100000")
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_postal_code" {
			t.Fatalf("postal context case %d not classified", i)
		}
	}

	t.Run("generic column stays Unknown", func(t *testing.T) {
		r := cnDetect(t, "c", 1, "100000")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("postal classified without context")
		}
	})
	t.Run("bad shapes", func(t *testing.T) {
		for i, value := range []string{"10000", "1000000", "10000a", "１２３４５６", "abcdef"} {
			r := cnDetect(t, "postal_code", 1, value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("postal shape case %d accepted", i)
			}
		}
	})
}

func TestCnUSCCKnownVectorAndStructure(t *testing.T) {
	// Hand-computed from GB 32100-2015 MOD 31: department/category 91, area
	// 440100, subject code 00000000A -> check character 3.
	const known = "9144010000000000A3"
	if got := genTestUSCC("91", "440100", "00000000A"); got != known {
		t.Fatal("USCC checksum generator disagrees with the first hand-computed vector")
	}
	// Second hand-computed vector with a different department/category, area,
	// and subject code.
	const known2 = "51110000123456789W"
	if got := genTestUSCC("51", "110000", "123456789"); got != known2 {
		t.Fatal("USCC checksum generator disagrees with the second hand-computed vector")
	}
	for i, value := range []string{known, known2} {
		r := cnDetect(t, "c", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_uscc" {
			t.Fatalf("known USCC vector %d not detected", i)
		}
	}
}

func TestCnUSCCPositives(t *testing.T) {
	cases := []string{
		genTestUSCC("91", "440100", "123456789"),
		genTestUSCC("51", "110000", "123456789"),
		genTestUSCC("93", "350100", "AB12CD34E"),
		genTestUSCC("Y1", "310000", "00000000A"),
	}
	for i, value := range cases {
		r := cnDetect(t, "c", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_uscc" {
			t.Fatalf("USCC positive case %d rejected", i)
		}
	}
}

func TestCnUSCCNegatives(t *testing.T) {
	cases := map[string]string{
		"checksum":          "9144010000000000A4",
		"excluded_charset":  "91440100000000I0A3",
		"lowercase":         "9144010000000000a3",
		"too_short":         "9144010000000000A",
		"too_long":          "9144010000000000A33",
		"unlisted_area":     genTestUSCC("91", "999999", "00000000A"),
		"bad_department":    genTestUSCC("99", "440100", "00000000A"),
		"category_mismatch": genTestUSCC("94", "440100", "00000000A"),
		"no_check_char":     "9144010000000000A",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			r := cnDetect(t, "c", 1, value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("USCC negative case %q accepted", name)
			}
		})
	}
}

func TestCnContextConfusionStaysUnknown(t *testing.T) {
	id := genTestID("110101", "19900101", 1)
	t.Run("phone column with ID values", func(t *testing.T) {
		r := cnDetect(t, "phone", 1, id)
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("confused phone column classified")
		}
		if !hasCode(evidenceByRule(t, r, "cn_mobile").Reasons, contracts.ConflictingEvidence) {
			t.Fatal("cn_mobile evidence missing conflicting evidence")
		}
	})
	t.Run("bank_card column with mobile values", func(t *testing.T) {
		r := cnDetect(t, "bank_card", 1, "13800138000")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("confused bank_card column classified")
		}
	})
	t.Run("id_card column with mobile values", func(t *testing.T) {
		r := cnDetect(t, "id_card", 1, "13800138000")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("confused id_card column classified")
		}
	})
}

func TestCnContradictoryContextStaysUnknown(t *testing.T) {
	card := genTestBankCard("6200000000000", 16)
	cases := []struct {
		name   string
		column string
		value  string
	}{
		{"bank_card_order_number", "bank_card_order_number", card},
		{"bank_transaction_id", "bank_transaction_id", card},
		{"手机号订单号", "手机号订单号", "13800138000"},
		{"联系电话流水号", "联系电话流水号", "13800138000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := cnDetect(t, tc.column, 1, tc.value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("contradictory order/transaction context %q classified", tc.name)
			}
		})
	}
}

func TestCnIDCardBankLuhnOverlap(t *testing.T) {
	const luhnID = "110101199001010250" // valid ID that also passes the Luhn check
	if !testLuhn(luhnID) {
		t.Fatal("overlap fixture is not Luhn-valid")
	}
	if testIDChecksum(luhnID[:17]) != luhnID[17] {
		t.Fatal("overlap fixture is not a checksum-valid ID")
	}
	// A generic column and an ID-named column have no disagreeing context, so
	// the context-free strong ID rule wins over the context-only bank format.
	for i, column := range []string{"c", "id_number"} {
		r := cnDetect(t, column, 1, luhnID)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_id_card" {
			t.Fatalf("ID/Luhn overlap column case %d not classified as cn_id_card", i)
		}
	}
	// Bank context conflicts with the ID format, so the column stays Unknown.
	r := cnDetect(t, "bank_card", 1, luhnID)
	if r.Sensitivity != contracts.Unknown {
		t.Fatal("ID/Luhn overlap under bank context was not held Unknown")
	}
	if !hasCode(evidenceByRule(t, r, "cn_id_card").Reasons, contracts.ConflictingEvidence) {
		t.Fatal("cn_id_card evidence missing conflicting evidence under bank context")
	}
}

func TestCnIDCardBankLuhnOverlapColumn(t *testing.T) {
	// 20 distinct IDs where exactly one also passes Luhn: the bank format ratio
	// stays far below the threshold, so the column is still cn_id_card.
	seqs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 250}
	values := make([]string, len(seqs))
	for i, seq := range seqs {
		values[i] = genTestID("110101", "19900101", seq)
	}
	r := cnDetect(t, "c", 0, values...)
	if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_id_card" {
		t.Fatal("20-distinct ID column with one Luhn overlap was not classified as cn_id_card")
	}
}

func TestCnNumericNonPIINegatives(t *testing.T) {
	cases := map[string][]string{
		"amount_generic_digits":  {"1234567890", "9876543210"},
		"quantity":               {"42", "7"},
		"unlisted_area_like_id":  {"999999199001010015"},
		"non_luhn_16_digits":     {"1234567890123456"},
		"six_digit_identifier":   {"123456"},
		"order_number_six_digit": {"100000", "100001"},
	}
	for column, values := range cases {
		t.Run(column, func(t *testing.T) {
			r := cnDetect(t, column, 1, values...)
			if r.Sensitivity == contracts.Sensitive {
				t.Fatalf("numeric non-PII column %q classified", column)
			}
		})
	}
}

func TestCnOrderNumberNotMobileAtThreshold(t *testing.T) {
	// 11-digit order numbers shaped like mobiles must stay Unknown without a
	// phone context even at the default 20-sample threshold.
	orders := make([]string, 20)
	for i := range orders {
		orders[i] = fmt.Sprintf("138%08d", i)
	}
	r := cnDetect(t, "order_number", 0, orders...)
	if r.Sensitivity != contracts.Unknown {
		t.Fatal("order_number column classified")
	}
}
