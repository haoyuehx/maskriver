package detect

import (
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Frozen synthetic pairs for issue #112. A positive means a person, even when
// the column is generic; negatives are ordinary words or non-person fields.
func TestCnPersonNameIssue112Evaluation(t *testing.T) {
	cases := []struct {
		column, value string
		person        bool
	}{
		{"姓名", "王明", true}, {"full_name", "李小明", true},
		{"name", "张伟", true}, {"name", "王芳", true},
		{"联系人", "陈大文", true}, {"姓名", "欧阳娜娜", true},
		{"name", "赵磊", true}, {"name", "黄敏", true},
		{"name", "李子", false}, {"name", "白天", false},
		{"name", "石头", false}, {"name", "王府", false},
		{"name", "林业", false}, {"name", "高铁", false},
		{"name", "周末", false}, {"name", "马路", false},
		{"product_name", "王明", false}, {"department_name", "张伟", false},
		{"name", "成都", false}, {"name", "普通文本", false},
	}
	var tp, fp, fn int
	for _, tc := range cases {
		r := cnDetect(t, tc.column, 1, tc.value)
		detected := r.Sensitivity == contracts.Sensitive && r.RuleID == "cn_person_name"
		if tc.person && detected {
			tp++
		}
		if tc.person && !detected {
			fn++
		}
		if !tc.person && detected {
			fp++
		}
	}
	t.Logf("issue112 fixed set: TP=%d FP=%d FN=%d", tp, fp, fn)
	if tp != 8 || fp != 0 || fn != 0 {
		t.Fatalf("issue112 acceptance: TP=%d FP=%d FN=%d", tp, fp, fn)
	}
}

func TestCnPersonNameIssue112SparseSampleStaysUnknown(t *testing.T) {
	// Production detection requires more evidence than one name-shaped value.
	r := cnDetect(t, "姓名", 0, "王明")
	if r.Sensitivity != contracts.Unknown {
		t.Fatalf("sparse name sample classified: %#v", r)
	}
}
