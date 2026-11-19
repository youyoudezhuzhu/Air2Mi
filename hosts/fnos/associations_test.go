package main

import (
	"strings"
	"testing"
)

func TestOneToOneRejectsDuplicateIncludingDisabledReceiver(t *testing.T) {
	inputs := []associationInput{{ID: "a", Name: "客厅", Enabled: false, Options: map[string]any{"speaker_id": "speaker"}}, {ID: "b", Name: "书房", Enabled: true, Options: map[string]any{"speaker_id": "speaker"}}}
	if err := validateOneToOne(inputs, nil); err == nil || !strings.Contains(err.Error(), "客厅") {
		t.Fatalf("duplicate accepted: %v", err)
	}
}
func TestOneToOneKeepsCurrentPairAndReassignmentReleasesOld(t *testing.T) {
	inputs := []associationInput{{ID: "a", Name: "A", Options: map[string]any{"speaker_id": "s1"}}, {ID: "b", Name: "B", Options: map[string]any{"speaker_id": "s2"}}}
	if err := validateOneToOne(inputs, []associationRoute{{ID: "r", Inputs: []string{"a"}, Speakers: []string{"s1"}}}); err != nil {
		t.Fatal(err)
	}
	inputs[0].Options = map[string]any{"speaker_id": "s3"}
	inputs[1].Options = map[string]any{"speaker_id": "s1"}
	if err := validateOneToOne(inputs, nil); err != nil {
		t.Fatal(err)
	}
}
func TestOneToOneCannotBeBypassedWithRoutes(t *testing.T) {
	inputs := []associationInput{{ID: "a", Name: "A", Options: map[string]any{"speaker_id": "s1"}}, {ID: "b", Name: "B"}}
	for _, routes := range [][]associationRoute{{{ID: "r", Inputs: []string{"b"}, Speakers: []string{"s1"}}}, {{ID: "r", Inputs: []string{"a"}, Speakers: []string{"s2"}}}, {{ID: "r", Inputs: []string{"a", "b"}, Speakers: []string{"s2"}}}} {
		if validateOneToOne(inputs, routes) == nil {
			t.Fatal("route bypass accepted")
		}
	}
}
