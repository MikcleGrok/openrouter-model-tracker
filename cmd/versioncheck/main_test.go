package main

import "testing"

func TestValidateTag(t *testing.T) {
	if got, err := validateTag([]string{"v1.2.3-rc.1"}); err != nil || got != "1.2.3-rc.1" {
		t.Fatalf("validateTag() = %q, %v", got, err)
	}
	if _, err := validateTag(nil); err == nil {
		t.Fatal("validateTag() without a tag succeeded")
	}
	if got, err := validateTag([]string{"--version", "1.2.3-rc.1"}); err != nil || got != "1.2.3-rc.1" {
		t.Fatalf("validateTag(--version) = %q, %v", got, err)
	}
	if _, err := validateTag([]string{"--version", "1.2.3+build"}); err == nil {
		t.Fatal("validateTag() accepted build metadata")
	}
}
