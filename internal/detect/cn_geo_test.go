package detect

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// This file tests the China-first geographic-name, personal-name, and address
// rules added for issue #6 with synthetic values only. Failure messages never
// print sample payloads.

func TestCnCityAliasesAndNormalization(t *testing.T) {
	cases := []struct {
		column string
		value  string
	}{
		{"city", "北京"},
		{"城市", "北京市"},
		{"city", "Beijing"},
		{"city", "BEIJING"},
		{"city", "Bei Jing"},
		{"province", "上海"},
		{"city", "Shanghai"},
		{"city", "广州"},
		{"city", "Guangzhou"},
		{"city", "深圳"},
		{"city", "Shenzhen"},
		{"city", "杭州"},
		{"city", "Hangzhou"},
		{"city", "西安"},
		{"city", "Xi'an"},
		{"city", "Xian"},
		{"city", "Xi’an"}, // curly apostrophe
		{"城市", "成都"},
		{"city", "Chengdu"},
		{"city", "重庆"},
		{"city", "Chongqing"},
		{"province", "广东省"},
		{"province", "Guangdong"},
		{"province", "内蒙古自治区"},
		{"province", "Inner Mongolia"},
		{"province", "广西壮族自治区"},
		{"province", "新疆维吾尔自治区"},
		{"province", "宁夏回族自治区"},
		{"province", "Tibet"},
		{"province", "Xizang"},
		{"城市", "GUANGZHOU CITY"},
		{"city", "Guangzhou City"},
	}
	for i, tc := range cases {
		r := cnDetect(t, tc.column, 1, tc.value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_city" {
			t.Fatalf("city positive case %d rejected", i)
		}
	}
}

func TestCnCityRequiredMappings(t *testing.T) {
	// The eight required Chinese<->pinyin city mappings and the regional suffix
	// variants for the bounded provinces and autonomous regions.
	cases := []struct {
		column string
		value  string
	}{
		{"city", "北京"}, {"city", "Beijing"},
		{"city", "上海"}, {"city", "Shanghai"},
		{"city", "广州"}, {"city", "Guangzhou"},
		{"city", "深圳"}, {"city", "Shenzhen"},
		{"city", "杭州"}, {"city", "Hangzhou"},
		{"city", "西安"}, {"city", "Xi'an"}, {"city", "Xian"},
		{"city", "成都"}, {"city", "Chengdu"},
		{"city", "重庆"}, {"city", "Chongqing"},
		{"province", "广西"}, {"province", "广西壮族自治区"}, {"province", "Guangxi"},
		{"province", "内蒙古"}, {"province", "内蒙古自治区"}, {"province", "Inner Mongolia"},
		{"province", "新疆"}, {"province", "新疆维吾尔自治区"}, {"province", "Xinjiang"},
		{"province", "宁夏"}, {"province", "宁夏回族自治区"}, {"province", "Ningxia"},
		{"province", "西藏"}, {"province", "西藏自治区"}, {"province", "Tibet"}, {"province", "Xizang"},
	}
	for i, tc := range cases {
		r := cnDetect(t, tc.column, 1, tc.value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_city" {
			t.Fatalf("required city mapping case %d rejected", i)
		}
	}
}

func TestCnCityNegativesAndOutOfCoverage(t *testing.T) {
	cases := map[string]string{
		"unsupported_city":       "武汉",
		"unsupported_pinyin":     "Wuhan",
		"unsupported_city2":      "南京",
		"unsupported_pinyin2":    "Nanjing",
		"foreign_city":           "Tokyo",
		"ordinary_word":          "marketing",
		"short_han":              "北",
		"admin_word_only":        "省",
		"fragment_with_district": "上海市浦东新区",
		"empty":                  "",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			r := cnDetect(t, "city", 1, value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("city negative case %q classified", name)
			}
			ev := evidenceByRule(t, r, "cn_city")
			if ev.Hits != 0 || ev.Eligible {
				t.Fatalf("city negative case %q produced a match", name)
			}
		})
	}
}

func TestCnCityRequiresContext(t *testing.T) {
	t.Run("generic column stays Unknown", func(t *testing.T) {
		r := cnDetect(t, "c", 1, "北京")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("city classified without context")
		}
		ev := evidenceByRule(t, r, "cn_city")
		if ev.Hits != 1 || ev.Eligible || !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
			t.Fatalf("cn_city evidence = %#v", ev)
		}
	})
	for _, column := range []string{"city", "城市", "province", "省份", "所在地区"} {
		t.Run("context "+column, func(t *testing.T) {
			r := cnDetect(t, column, 1, "北京")
			if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_city" {
				t.Fatalf("city context %q not classified", column)
			}
		})
	}
}

func TestCnCityFieldConflictStaysUnknown(t *testing.T) {
	t.Run("address column with a city value", func(t *testing.T) {
		r := cnDetect(t, "地址", 1, "北京")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("city value under address context classified")
		}
	})
	t.Run("name column with a city value", func(t *testing.T) {
		r := cnDetect(t, "name", 1, "北京")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("city value under name context classified")
		}
	})
	t.Run("city column with an address value", func(t *testing.T) {
		r := cnDetect(t, "city", 1, "北京市朝阳区建国路88号")
		if r.Sensitivity != contracts.Unknown {
			t.Fatal("address value under city context classified")
		}
		if !hasCode(evidenceByRule(t, r, "cn_address").Reasons, contracts.ConflictingEvidence) {
			t.Fatal("cn_address evidence missing conflicting_evidence")
		}
	})
}

func TestCnPersonNamePositives(t *testing.T) {
	cases := []struct {
		column string
		value  string
	}{
		{"姓名", "王明"},
		{"full_name", "李小明"},
		{"name", "王小明华"}, // four Han with a listed single surname
		{"姓名", "欧阳娜娜"},   // listed compound surname
		{"name", "司马光"},
		{"名字", "赵四"},
		{"联系人", "陈大文"},
	}
	for i, tc := range cases {
		r := cnDetect(t, tc.column, 1, tc.value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_person_name" {
			t.Fatalf("person-name positive case %d rejected", i)
		}
	}
}

func TestCnPersonNameNegatives(t *testing.T) {
	cases := map[string]string{
		"ordinary_text":  "普通文本", // four Han but no listed leading surname
		"ordinary_word":  "数据",
		"city_value":     "北京",
		"pinyin":         "marketing",
		"pinyin_name":    "wang ming",
		"single_char":    "张",
		"compound_alone": "欧阳",
		"digits":         "12345",
		"too_long":       "王小明华强",
		"latin_plus_han": "王Ming",
		"empty":          "",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			r := cnDetect(t, "姓名", 1, value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("person-name negative case %q classified as %q", name, r.RuleID)
			}
		})
	}
}

func TestCnPersonNameRequiresContext(t *testing.T) {
	r := cnDetect(t, "c", 1, "王明")
	if r.Sensitivity != contracts.Unknown {
		t.Fatal("person name classified without context")
	}
	ev := evidenceByRule(t, r, "cn_person_name")
	if ev.Hits != 1 || ev.Eligible || !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
		t.Fatalf("cn_person_name evidence = %#v", ev)
	}
}

func TestCnPersonNameDeclinesGeographicAlias(t *testing.T) {
	// 成 is a listed surname and 成都 is a bounded geographic alias, so under a
	// name column the two claims cannot be distinguished and the column stays
	// Unknown instead of producing a known false positive.
	r := cnDetect(t, "name", 1, "成都")
	if r.Sensitivity != contracts.Unknown {
		t.Fatalf("geographic alias under a name column classified: %#v", r)
	}
	// The same value in a city column resolves to the city rule.
	if r := cnDetect(t, "city", 1, "成都"); r.RuleID != "cn_city" {
		t.Fatalf("city value under city context = %#v", r)
	}
}

func TestCnPersonNameSurnameLeadingWordsDocumented(t *testing.T) {
	// These ordinary words start with listed surnames (李/白/石) and satisfy the
	// two-Han shape, so they remain measured false positives under a name column.
	// The synthetic evaluation counts them rather than hiding them.
	for _, value := range []string{"李子", "白天", "石头"} {
		r := cnDetect(t, "name", 1, value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_person_name" {
			t.Fatalf("documented surname-leading ambiguity changed: %#v", r)
		}
	}
}

func TestCnAddressPositives(t *testing.T) {
	cases := []struct {
		column string
		value  string
	}{
		{"地址", "北京市朝阳区建国路88号"},
		{"addr", "上海市浦东新区世纪大道100号"},
		{"收货地址", "广东省深圳市南山区科技园南路15号3栋201室"},
		{"地址", "建国路88号"},
		{"addr", "中关村大街1号院3号楼"},
		{"地址", "华夏大厦5层501室"},
		{"addr", "三里屯路19号"},
		{"addr", "二七路1号"},
		{"住址", "浙江省杭州市西湖区文三路15号"},
	}
	for i, tc := range cases {
		r := cnDetect(t, tc.column, 1, tc.value)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_address" {
			t.Fatalf("address positive case %d rejected", i)
		}
	}
}

func TestCnAddressInsufficientAndNegatives(t *testing.T) {
	cases := map[string]string{
		"admin_only":          "北京市",
		"admin_two_levels":    "上海市浦东新区",
		"road_without_number": "北京市朝阳区建国路",
		"road_word_only":      "这是一条路",
		"ordinary_text":       "今天天气很好",
		"room_only":           "3栋2单元501室",
		"english_street":      "NO.123 Main Street",
		"notice_admin_road":   "城市道路管理",
		"notice_house_prose":  "社区1号公告",
		"notice_street":       "街道通知",
		"notice_sentence":     "关于加强管理的通知",
		"descriptive_clause":  "他从1号楼出来",
		"notice_document":     "公告第1号",
		"ascii_stem_road":     "ABC路1号",
		"punctuation_road":    "!!路1号",
		"admin_without_stem":  "区县1号",
		"standalone_unit":     "栋501室",
		"empty":               "",
		"too_short":           "北京",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			r := cnDetect(t, "地址", 1, value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("address negative case %q classified as %q", name, r.RuleID)
			}
		})
	}
}

func TestCnAddressContextForms(t *testing.T) {
	for _, column := range []string{"address", "addr", "street", "home_address", "residential_address", "地址", "住址", "收货地址"} {
		t.Run(column, func(t *testing.T) {
			r := cnDetect(t, column, 1, "北京市朝阳区建国路88号")
			if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_address" {
				t.Fatalf("address context %q not classified", column)
			}
		})
	}
}

func TestCnAddressSpecificContextExcluded(t *testing.T) {
	// email_address and ip_address are more specific contexts, so the generic
	// address token must not turn them into a conflicting address claim.
	t.Run("email_address with emails", func(t *testing.T) {
		r := cnDetect(t, "email_address", 1, "user01@example.com")
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "email" {
			t.Fatalf("email_address now conflicts with the address rule: %#v", r)
		}
	})
	t.Run("ip_address with IPs", func(t *testing.T) {
		r := cnDetect(t, "ip_address", 1, "192.168.10.20")
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "ip_address" {
			t.Fatalf("ip_address now conflicts with the address rule: %#v", r)
		}
	})
	t.Run("email_address with a Chinese address", func(t *testing.T) {
		r := cnDetect(t, "email_address", 1, "北京市朝阳区建国路88号")
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("conflicting address under email_address classified: %#v", r)
		}
	})
}

func TestCnAddressRequiresContext(t *testing.T) {
	r := cnDetect(t, "c", 1, "北京市朝阳区建国路88号")
	if r.Sensitivity != contracts.Unknown {
		t.Fatal("address classified without context")
	}
	ev := evidenceByRule(t, r, "cn_address")
	if ev.Hits != 1 || ev.Eligible || !hasCode(ev.Reasons, contracts.InsufficientEvidence) {
		t.Fatalf("cn_address evidence = %#v", ev)
	}
}

func TestCnMultipleEligibleRulesStayUnknown(t *testing.T) {
	// A column that claims both a personal-name field and a geographic field,
	// holding a value that satisfies both bounded rules, must stay Unknown.
	r := cnDetect(t, "姓名城市", 1, "成都")
	if r.Sensitivity != contracts.Unknown {
		t.Fatalf("multiple eligible rules did not stay Unknown: %#v", r)
	}
	if !hasCode(evidenceByRule(t, r, "cn_city").Reasons, contracts.ConflictingEvidence) {
		t.Fatal("cn_city evidence missing conflicting_evidence")
	}
	if !hasCode(evidenceByRule(t, r, "cn_person_name").Reasons, contracts.ConflictingEvidence) {
		t.Fatal("cn_person_name evidence missing conflicting_evidence")
	}
}

func TestCnGeoRulesDoNotLeakRawSamples(t *testing.T) {
	cases := map[string]struct{ column, value string }{
		"cn_city":        {"city", "北京"},
		"cn_person_name": {"姓名", "王明"},
		"cn_address":     {"地址", "北京市朝阳区建国路88号"},
	}
	for ruleID, tc := range cases {
		values := textValues(t, tc.value)
		req := testRequest(values)
		req.Column.Ref.Column = tc.column
		req.MinSamples = 1
		r := detectOK(t, req)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != ruleID {
			t.Fatalf("leak fixture for %s did not classify", ruleID)
		}
		decisionJSON, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal decision for %s: %v", ruleID, err)
		}
		if strings.Contains(string(decisionJSON), tc.value) {
			t.Fatalf("decision for %s leaked the raw sample", ruleID)
		}
		valueJSON, err := json.Marshal(values[0])
		if err != nil {
			t.Fatalf("marshal value for %s: %v", ruleID, err)
		}
		if strings.Contains(string(valueJSON), tc.value) {
			t.Fatalf("value JSON for %s leaked the raw sample", ruleID)
		}
	}
}

func TestCnGeoColumnAggregationDefaultThreshold(t *testing.T) {
	cities := []string{
		"北京", "上海", "广州", "深圳", "杭州", "西安", "成都", "重庆",
		"广东", "浙江", "江苏", "四川", "陕西", "湖北", "福建", "山东",
		"河南", "河北", "湖南", "安徽",
	}
	t.Run("20 distinct city values meet the default MinSamples", func(t *testing.T) {
		r := cnDetect(t, "city", 0, cities...)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_city" {
			t.Fatalf("20 distinct city values = %#v", r)
		}
	})
	t.Run("19 distinct city values stay Unknown below MinSamples", func(t *testing.T) {
		r := cnDetect(t, "city", 0, cities[:19]...)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("19 distinct city values = %#v", r)
		}
	})
	t.Run("18 of 20 city values meet the 90 percent threshold", func(t *testing.T) {
		values := append([]string{}, cities[:18]...)
		values = append(values, fillers(2)...)
		r := cnDetect(t, "city", 0, values...)
		if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_city" {
			t.Fatalf("18/20 city values = %#v", r)
		}
	})
	t.Run("17 of 20 city values stay Unknown below the threshold", func(t *testing.T) {
		values := append([]string{}, cities[:17]...)
		values = append(values, fillers(3)...)
		r := cnDetect(t, "city", 0, values...)
		if r.Sensitivity != contracts.Unknown {
			t.Fatalf("17/20 city values = %#v", r)
		}
	})
}
