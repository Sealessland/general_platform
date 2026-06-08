package domain

import "testing"

func TestCloneHelpersReturnIndependentCopies(t *testing.T) {
	strings := []string{"one", "two"}
	clonedStrings := CloneStringSlice(strings)
	clonedStrings[0] = "changed"
	if strings[0] != "one" {
		t.Fatalf("CloneStringSlice returned aliased slice: %+v", strings)
	}

	ints := []int64{1, 2}
	clonedInts := CloneInt64Slice(ints)
	clonedInts[0] = 99
	if ints[0] != 1 {
		t.Fatalf("CloneInt64Slice returned aliased slice: %+v", ints)
	}

	values := map[string]string{"color": "red"}
	clonedMap := CloneMap(values)
	clonedMap["color"] = "blue"
	if values["color"] != "red" {
		t.Fatalf("CloneMap returned aliased map: %+v", values)
	}
}

func TestCloneHelpersPreserveNilForEmptyInputs(t *testing.T) {
	if CloneStringSlice(nil) != nil {
		t.Fatal("expected nil string slice clone")
	}
	if CloneInt64Slice(nil) != nil {
		t.Fatal("expected nil int64 slice clone")
	}
	if CloneMap(nil) != nil {
		t.Fatal("expected nil map clone")
	}
}
