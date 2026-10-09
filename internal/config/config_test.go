package config

import "testing"

func TestDryRun(t *testing.T) {
	if !(Options{}).DryRun() {
		t.Fatal("zero value must be dry run")
	}
	if (Options{Apply: true}).DryRun() {
		t.Fatal("explicit apply must express write intent")
	}
}
