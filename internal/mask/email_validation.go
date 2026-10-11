package mask

import "strings"

// validMaskEmail is a bounded ASCII email syntax check adapted from the
// alternative implementation contributed in PR #26. It does not validate
// mailbox existence, DNS or full internationalized email syntax.
func validMaskEmail(value string) bool {
	if value == "" || len(value) > 254 ||
		strings.TrimSpace(value) != value || strings.Count(value, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(value, "@")
	if local == "" || len(local) > 64 || domain == "" || len(domain) > 253 ||
		strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") ||
		strings.Contains(local, "..") {
		return false
	}
	const localPunctuation = ".!#$%&'*+/=?^`{|}~-"
	for i := 0; i < len(local); i++ {
		c := local[i]
		if !maskASCIIAlphanumeric(c) && !strings.ContainsRune(localPunctuation, rune(c)) {
			return false
		}
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if !maskASCIIAlphanumeric(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func maskASCIIAlphanumeric(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
