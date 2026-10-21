package internal_test

import (
	"errors"
	"testing"

	"notebook/test/e2e/internal"
)

func TestCheckPrerequisites_AllPresent_Succeeds(t *testing.T) {
	lookPath := func(bin string) (string, error) { return "/usr/bin/" + bin, nil }
	if err := internal.CheckPrerequisites(lookPath); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckPrerequisites_MissingBinary_ListsIt(t *testing.T) {
	lookPath := func(bin string) (string, error) {
		if bin == "kind" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + bin, nil
	}
	err := internal.CheckPrerequisites(lookPath)
	if err == nil {
		t.Fatalf("expected an error listing the missing binary")
	}
	if !contains(err.Error(), "kind") {
		t.Fatalf("expected error to mention 'kind', got: %v", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
