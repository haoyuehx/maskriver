package mask

import (
	"crypto/sha256"
	"testing"
)

// Fuzz cases use synthetic fixtures and ensure that checksum-valid generated
// values remain within the supported bounded shape. They do not validate real
// personal identities, account ownership, or anonymity.
func FuzzCNIDReplacement(f *testing.F) {
	f.Add(cnSyntheticID())
	f.Add("110101202302300010")
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 30 || !cnValidID(value) {
			return
		}
		seed := sha256.Sum256([]byte("synthetic-fuzz-key"))
		got, err := replaceCNID(value, seed)
		if err != nil || got == value || !cnValidID(got) {
			t.Fatal("CN ID format and checksum invariant failed")
		}
	})
}

func FuzzCNBankReplacement(f *testing.F) {
	f.Add(cnSyntheticBank())
	f.Add("1234")
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 30 || !cnValidBank(value) {
			return
		}
		seed := sha256.Sum256([]byte("synthetic-fuzz-key"))
		got, err := replaceCNBank(value, seed)
		if err != nil || got == value || !cnValidBank(got) || len(got) != len(value) {
			t.Fatal("bank length or checksum invariant failed")
		}
	})
}

func FuzzCNUSCCReplacement(f *testing.F) {
	f.Add(cnSyntheticUSCC())
	f.Add("invalid")
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 30 || !cnValidUSCC(value) {
			return
		}
		seed := sha256.Sum256([]byte("synthetic-fuzz-key"))
		got, err := replaceCNUSCC(value, seed)
		if err != nil || got == value || !cnValidUSCC(got) {
			t.Fatal("USCC format and checksum invariant failed")
		}
	})
}
