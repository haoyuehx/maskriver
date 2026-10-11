package mask

import (
	"strings"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This module deliberately mirrors only the bounded public shapes accepted by
// the China-first detector. Lists are not comprehensive or issuance evidence.
const cnIDAreas = "|110101|110105|110108|120101|310101|310115|440103|440106|" +
	"440304|330106|320102|510107|420106|610113|500106|"
const cnUSCCAreas = "|110000|120000|310000|500000|440100|440300|330100|" +
	"320100|510100|420100|610100|350100|" + cnIDAreas[1:]
const cnMobileSegments = "|130|131|132|133|134|135|136|137|138|139|" +
	"150|151|152|153|155|156|157|158|159|" +
	"170|171|173|175|176|177|178|" +
	"180|181|182|183|184|185|186|187|188|189|" +
	"191|193|195|196|198|199|"
const cnIDWeights = "0709100508040201060307091005080402"
const cnIDChecks = "10X98765432"
const cnUSCCAlphabet = "0123456789ABCDEFGHJKLMNPQRTUWXY"
const cnUSCCWeights = "0103092719261617202925130824103028"
const cnUSCCCategories = "|11|51|52|53|59|91|92|93|Y1|"

func cnAllowed(list, value string) bool {
	return strings.Contains(list, "|"+value+"|")
}

func cnIDCheck(digits string) byte {
	sum := 0
	for i := 0; i < 17; i++ {
		weight := int(cnIDWeights[2*i]-'0')*10 + int(cnIDWeights[2*i+1]-'0')
		sum += int(digits[i]-'0') * weight
	}
	return cnIDChecks[sum%11]
}

func cnValidID(s string) bool {
	if len(s) != 18 || !cnASCII(s[:17]) ||
		!cnAllowed(cnIDAreas, s[:6]) ||
		!cnDateValid(s[6:14], "20060102") || s[14:17] == "000" {
		return false
	}
	last := s[17]
	if last == 'x' {
		last = 'X'
	}
	return last == cnIDCheck(s[:17])
}

func replaceCNID(source string, seed [32]byte) (string, error) {
	if !cnValidID(source) {
		return "", contracts.ErrInvalid
	}
	areas := [...]string{"110101", "310101", "440103"}
	for attempt := 0; attempt < 8; attempt++ {
		tag := cnDecimal(attempt)
		date := time.Date(
			1980+int(cnNumber(seed, "id_year_"+tag, 26)),
			time.Month(1+cnNumber(seed, "id_month_"+tag, 12)),
			1+int(cnNumber(seed, "id_day_"+tag, 28)), 0, 0, 0, 0, time.UTC,
		)
		sequence := 1 + cnNumber(seed, "id_sequence_"+tag, 999)
		first17 := areas[cnNumber(seed, "id_area_"+tag, uint64(len(areas)))] +
			date.Format("20060102") + paddedCN(sequence, 3)
		output := first17 + string(cnIDCheck(first17))
		if output != source {
			return output, nil
		}
	}
	return "", contracts.ErrUnsafe
}

func paddedCN(n uint64, width int) string {
	s := cnDecimal(int(n))
	return strings.Repeat("0", width-len(s)) + s
}

func cnMobileParts(s string) (prefix string, digits string, ok bool) {
	switch {
	case strings.HasPrefix(s, "+86"):
		prefix, s = "+86", s[3:]
	case strings.HasPrefix(s, "86"):
		prefix, s = "86", s[2:]
	}
	return prefix, s, len(s) == 11 && cnASCII(s) && cnAllowed(cnMobileSegments, s[:3])
}

func replaceCNMobile(source string, seed [32]byte) (string, error) {
	country, _, ok := cnMobileParts(source)
	if !ok {
		return "", contracts.ErrInvalid
	}
	prefixes := [...]string{"139", "188", "150", "176"}
	for attempt := 0; attempt < 8; attempt++ {
		tag := cnDecimal(attempt)
		output := country + prefixes[cnNumber(seed, "mobile_prefix_"+tag, uint64(len(prefixes)))] +
			cnDigits(seed, "mobile_tail_"+tag, 8)
		if output != source {
			return output, nil
		}
	}
	return "", contracts.ErrUnsafe
}

func cnLuhnDigit(first string) byte {
	sum, double := 0, true
	for i := len(first) - 1; i >= 0; i-- {
		d := int(first[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return byte('0' + (10-sum%10)%10)
}

func cnValidBank(s string) bool {
	return len(s) >= 16 && len(s) <= 19 && cnASCII(s) &&
		s[0] != '0' && s[len(s)-1] == cnLuhnDigit(s[:len(s)-1])
}

func replaceCNBank(source string, seed [32]byte) (string, error) {
	if !cnValidBank(source) {
		return "", contracts.ErrInvalid
	}
	for attempt := 0; attempt < 8; attempt++ {
		tag := cnDecimal(attempt)
		first := string(byte('1'+cnNumber(seed, "bank_first_"+tag, 9))) +
			cnDigits(seed, "bank_body_"+tag, len(source)-2)
		output := first + string(cnLuhnDigit(first))
		if output != source {
			return output, nil
		}
	}
	return "", contracts.ErrUnsafe
}

func cnUSCCCheck(s string) byte {
	sum := 0
	for i := 0; i < 17; i++ {
		weight := int(cnUSCCWeights[2*i]-'0')*10 + int(cnUSCCWeights[2*i+1]-'0')
		sum += strings.IndexByte(cnUSCCAlphabet, s[i]) * weight
	}
	return cnUSCCAlphabet[(31-sum%31)%31]
}

func cnValidUSCC(s string) bool {
	if len(s) != 18 || !cnAllowed(cnUSCCCategories, s[:2]) ||
		!cnAllowed(cnUSCCAreas, s[2:8]) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(cnUSCCAlphabet, s[i]) == -1 {
			return false
		}
	}
	return s[17] == cnUSCCCheck(s[:17])
}

func replaceCNUSCC(source string, seed [32]byte) (string, error) {
	if !cnValidUSCC(source) {
		return "", contracts.ErrInvalid
	}
	areas := [...]string{"110000", "310000", "440300"}
	for attempt := 0; attempt < 8; attempt++ {
		tag := cnDecimal(attempt)
		var subject strings.Builder
		for i := 0; i < 9; i++ {
			subject.WriteByte(cnUSCCAlphabet[cnNumber(seed, "uscc_"+tag+"_"+cnDecimal(i), uint64(len(cnUSCCAlphabet)))])
		}
		first17 := "91" + areas[cnNumber(seed, "uscc_area_"+tag, uint64(len(areas)))] + subject.String()
		output := first17 + string(cnUSCCCheck(first17))
		if output != source {
			return output, nil
		}
	}
	return "", contracts.ErrUnsafe
}

func cnValidPostal(s string) bool { return len(s) == 6 && cnASCII(s) }

func replaceCNPostal(source string, seed [32]byte) (string, error) {
	if !cnValidPostal(source) {
		return "", contracts.ErrInvalid
	}
	for attempt := 0; attempt < 8; attempt++ {
		tag := cnDecimal(attempt)
		output := cnDigits(seed, "postal_"+tag, 6)
		if output != source {
			return output, nil
		}
	}
	return "", contracts.ErrUnsafe
}
