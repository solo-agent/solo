package service

import (
	"testing"
)

func TestValidRelTypes_Item1(t *testing.T) {
	// After migration 000036: only 2 types valid.
	expected := []string{"assigns_to", "collaborates_with"}
	for _, rt := range expected {
		if !ValidRelTypes[rt] {
			t.Errorf("expected %s to be valid", rt)
		}
	}
	removed := []string{"reports_to", "delegates_to", "escalates_to"}
	for _, rt := range removed {
		if ValidRelTypes[rt] {
			t.Errorf("expected %s to be removed post-migration", rt)
		}
	}
}
