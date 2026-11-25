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

func TestCnPersonNameIssue112PersonRolesOverrideEntityContext(t *testing.T) {
	for _, tc := range []struct{ column, value string }{
		{"department_manager_name", "王明"},
		{"department_contact_name", "李小明"},
		{"product_owner_name", "王芳"},
		{"product_manager_name", "陈大文"},
		{"department_manager_full_name", "王明"},
		{"department_contact_first_name", "李小明"},
		{"product_owner_full_name", "王芳"},
		{"product_manager_display_name", "陈大文"},
		{"department_head_name", "王明"},
		{"department_director_name", "李小明"},
		{"product_assignee_name", "王芳"},
		{"department_supervisor_name", "陈大文"},
		{"产品经理姓名", "张伟"},
		{"部门负责人姓名", "王明"},
		{"部门联系人", "李小明"},
		{"部门员工名字", "王明"},
		{"产品经理名字", "张伟"},
		{"部门人名", "王明"},
		{"产品负责人人名", "李小明"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			r := cnDetect(t, tc.column, 1, tc.value)
			if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_person_name" {
				t.Fatalf("person-role column must classify as a name: %#v", r)
			}
		})
	}
	for _, tc := range []struct{ column, value string }{
		{"product_name", "王明"},
		{"department_name", "张伟"},
		{"employee_department_name", "李小明"},
		{"manager_product_name", "陈大文"},
		{"owner_product_name", "王芳"},
		{"员工部门姓名", "李小明"},
		{"经理产品姓名", "陈大文"},
		{"员工部门名字", "王明"},
		{"员工产品名字", "张伟"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			r := cnDetect(t, tc.column, 1, tc.value)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("entity-name column must remain unknown: %#v", r)
			}
		})
	}
}

func TestCnPersonNameIssue112PersonRolesAtProductionThreshold(t *testing.T) {
	// Twenty distinct synthetic samples exercise the default MinSamples=20
	// and MinRatio=.9 gates. Eighteen are name-shaped; two are ordinary words.
	values := []string{
		"王明", "李小明", "张伟", "刘洋", "陈静", "杨帆", "赵磊", "黄敏", "吴强",
		"周杰", "徐亮", "孙晨", "马超", "朱琳", "胡军", "郭伟", "何芳", "罗敏",
		"普通文本", "测试词语",
	}
	for _, column := range []string{
		"department_manager_name", "product_owner_name", "部门负责人姓名", "部门联系人",
		"department_manager_full_name", "department_contact_first_name",
		"product_owner_full_name", "product_manager_display_name",
		"department_head_name", "department_director_name", "product_assignee_name",
		"部门员工名字", "产品经理名字",
		"部门人名", "产品负责人人名",
	} {
		t.Run(column, func(t *testing.T) {
			r := cnDetect(t, column, 0, values...)
			if r.Sensitivity != contracts.Sensitive || r.RuleID != "cn_person_name" {
				t.Fatalf("default-threshold person field must remain sensitive: %#v", r)
			}
		})
	}
	for _, column := range []string{
		"product_name", "department_name", "employee_department_name",
		"manager_product_name", "owner_product_name",
		"员工部门姓名", "经理产品姓名", "员工部门名字", "员工产品名字",
	} {
		t.Run(column, func(t *testing.T) {
			r := cnDetect(t, column, 0, values...)
			if r.Sensitivity != contracts.Unknown {
				t.Fatalf("default-threshold entity name must remain unknown: %#v", r)
			}
		})
	}
}
