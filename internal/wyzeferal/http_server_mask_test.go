package wyzeferal

import "testing"

func TestIsMaskedWyzeAPIKey(t *testing.T) {
	t.Parallel()
	if !isMaskedWyzeAPIKey("abcd…wxyz") {
		t.Fatal("ellipsis mask")
	}
	if !isMaskedWyzeAPIKey("****") {
		t.Fatal("stars")
	}
	if isMaskedWyzeAPIKey("real-key-without-ellipsis-12345678901234567890") {
		t.Fatal("real key should not be masked")
	}
}
