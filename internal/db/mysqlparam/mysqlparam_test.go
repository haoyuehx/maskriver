package mysqlparam

import "testing"

func TestUnsafeNormalizesScopeAliases(t *testing.T) {
	for _, key := range []string{
		"foreign_key_checks", "@@session.transaction_read_only",
		"@@local.foreign_key_checks", "SESSION transaction_read_only",
		"GLOBAL sql_mode", "unique_checks", "check_constraint_checks",
	} {
		if !Unsafe(key, "0") {
			t.Errorf("Unsafe(%q) = false, want true", key)
		}
	}
}

func TestUnsafeRejectsSetInjection(t *testing.T) {
	for _, value := range []string{"UTC,foreign_key_checks=0", "x;sql_mode=", "a,b"} {
		if !Unsafe("time_zone", value) {
			t.Errorf("Unsafe value %q accepted", value)
		}
	}
}

func TestUnsafeAllowsNormalOptions(t *testing.T) {
	for _, key := range []string{"time_zone", "charset", "collation", "tls", "readTimeout"} {
		if Unsafe(key, "UTC") {
			t.Errorf("Unsafe(%q) = true, want false", key)
		}
	}
}
