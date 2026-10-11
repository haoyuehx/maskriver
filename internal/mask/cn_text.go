package mask

import (
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

type cnCityPair struct {
	han    string
	pinyin string
}

var cnCities = [...]cnCityPair{
	{"北京", "beijing"}, {"上海", "shanghai"}, {"广州", "guangzhou"},
	{"深圳", "shenzhen"}, {"杭州", "hangzhou"}, {"成都", "chengdu"},
	{"重庆", "chongqing"},
}

// cnCityStyle accepts a small, explicit dictionary with no generalized
// transliteration. Chinese administrative suffix and pinyin case are retained.
func cnCityStyle(source string) (style int, suffix string, ok bool) {
	base := source
	switch {
	case strings.HasSuffix(base, "市"):
		suffix = "市"
		base = strings.TrimSuffix(base, "市")
	case strings.HasSuffix(strings.ToLower(base), " city"):
		suffix = source[len(source)-5:]
		base = source[:len(source)-5]
	}
	for _, city := range cnCities {
		if base == city.han {
			if suffix != "" && suffix != "市" {
				return 0, "", false
			}
			return 0, suffix, true
		}
		if strings.EqualFold(base, city.pinyin) {
			if suffix == "市" {
				return 0, "", false
			}
			if len(base) > 0 && base[0] >= 'A' && base[0] <= 'Z' {
				return 2, suffix, true
			}
			return 1, suffix, true
		}
	}
	return 0, "", false
}

func replaceCNCity(source string, seed [32]byte) (string, error) {
	style, suffix, ok := cnCityStyle(source)
	if !ok {
		return "", contracts.ErrUnsupported
	}
	for i := 0; i < len(cnCities); i++ {
		city := cnCities[(int(cnNumber(seed, "city", uint64(len(cnCities))))+i)%len(cnCities)]
		out := city.han
		if style > 0 {
			out = city.pinyin
			if style == 2 {
				out = strings.ToUpper(out[:1]) + out[1:]
			}
		}
		if out += suffix; out != source {
			return out, nil
		}
	}
	return "", contracts.ErrUnsafe
}

const cnSurnames = "王李张刘陈杨黄赵吴周徐孙马朱胡郭何高林"
const cnCompoundSurnames = "|欧阳|司马|上官|诸葛|"

func cnNameShape(source string) (compound bool, ok bool) {
	if !cnHan(source, 2, 4) {
		return false, false
	}
	runes := []rune(source)
	if len(runes) >= 3 && cnAllowed(cnCompoundSurnames, string(runes[:2])) {
		return true, true
	}
	return false, strings.ContainsRune(cnSurnames, runes[0])
}

func replaceCNName(source string, seed [32]byte) (string, error) {
	compound, ok := cnNameShape(source)
	if !ok {
		return "", contracts.ErrUnsupported
	}
	runeCount := utf8.RuneCountInString(source)
	surnames := []rune(cnSurnames)
	given := []rune("星河明月清云华光秋宁安")
	for attempt := 0; attempt < 8; attempt++ {
		tag := strconv.Itoa(attempt)
		surname := string(surnames[cnNumber(seed, "surname_"+tag, uint64(len(surnames)))])
		if compound {
			choices := [...]string{"欧阳", "司马", "上官", "诸葛"}
			surname = choices[cnNumber(seed, "compound_"+tag, uint64(len(choices)))]
		}
		var b strings.Builder
		b.WriteString(surname)
		for i := 0; i < runeCount-utf8.RuneCountInString(surname); i++ {
			b.WriteRune(given[cnNumber(seed, "given_"+tag+"_"+strconv.Itoa(i), uint64(len(given)))])
		}
		if out := b.String(); out != source {
			return out, nil
		}
	}
	return "", contracts.ErrUnsafe
}

func replaceCNAddress(source string, seed [32]byte) (string, error) {
	if !cnAddressShape.MatchString(source) || strings.IndexFunc(source, unicode.IsControl) >= 0 {
		return "", contracts.ErrUnsupported
	}
	// "虚构路" intentionally signals a synthetic street stem; an actual
	// existence or deliverability check is neither performed nor implied.
	streets := [...]string{"示例路", "虚构街", "样本巷"}
	for attempt := 0; attempt < 8; attempt++ {
		tag := strconv.Itoa(attempt)
		street := streets[cnNumber(seed, "street_"+tag, uint64(len(streets)))]
		number := 1 + cnNumber(seed, "house_"+tag, 9999)
		out := "合成区" + street + strconv.Itoa(int(number)) + "号"
		if out != source {
			return out, nil
		}
	}
	return "", contracts.ErrUnsafe
}

func cnParseDate(s string, kind contracts.Kind) (time.Time, string, error) {
	layouts := [...]string{"2006-01-02", "2006/01/02", "2006年1月2日"}
	for _, layout := range layouts {
		if kind == contracts.Date && layout != time.DateOnly {
			continue
		}
		parsed, err := time.Parse(layout, s)
		if err == nil && parsed.Format(layout) == s && parsed.Year() >= 1900 {
			return parsed, layout, nil
		}
	}
	return time.Time{}, "", contracts.ErrUnsupported
}

func replaceCNDate(source string, seed [32]byte, kind contracts.Kind) (string, error) {
	_, layout, err := cnParseDate(source, kind)
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 8; attempt++ {
		tag := strconv.Itoa(attempt)
		day := 1 + cnNumber(seed, "date_"+tag, 365*24)
		date := time.Date(1995, time.January, 1+int(day), 0, 0, 0, 0, time.UTC)
		if out := date.Format(layout); out != source {
			return out, nil
		}
	}
	return "", contracts.ErrUnsafe
}
