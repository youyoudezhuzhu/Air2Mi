package main

import (
	"errors"
	"testing"
)

func TestDiscoveryImportPreservesRenamesAndDoesNotDuplicate(t *testing.T) {
	found := []discoveredSpeaker{{ID: "a", Name: "Original", IP: "192.0.2.1"}, {ID: "b", Name: "New", IP: "192.0.2.2"}, {ID: "b", Name: "New", IP: "192.0.2.2"}, {ID: "changed", Name: "Old device", IP: "192.0.2.1"}}
	known := []discoveredSpeaker{{ID: "a", Name: "用户改名的厨房", IP: "192.0.2.1"}}
	var created []discoveredSpeaker
	added, skipped, err := importDiscovered(found, known, func(s discoveredSpeaker) error { created = append(created, s); return nil })
	if err != nil || added != 1 || skipped != 3 || len(created) != 1 || created[0].ID != "b" {
		t.Fatalf("%d %d %v %+v", added, skipped, err, created)
	}
	if known[0].Name != "用户改名的厨房" {
		t.Fatal("user rename overwritten")
	}
}
func TestDiscoveryImportReportsPartialFailure(t *testing.T) {
	found := []discoveredSpeaker{{ID: "a", Name: "A", IP: "192.0.2.1"}, {ID: "b", Name: "B", IP: "192.0.2.2"}}
	calls := 0
	added, _, err := importDiscovered(found, nil, func(s discoveredSpeaker) error {
		calls++
		if calls == 2 {
			return errors.New("real failure")
		}
		return nil
	})
	if err == nil || added != 1 {
		t.Fatal("failure hidden")
	}
}

func TestDiscoveryRepairsByStableDiscoveryIdentity(t *testing.T) {
	found := []discoveredSpeaker{
		{ID: "living", Name: "Living", IP: "192.0.2.18", Model: "937E4BA6", Options: map[string]any{"discovery_id": "mac-living"}},
		{ID: "new-id-kitchen", Name: "Mi Speaker", IP: "192.0.2.116", Model: "LX06-2", Options: map[string]any{"discovery_id": "uuid-kitchen"}},
		{ID: "new-id-study", Name: "Mi Speaker 2", IP: "192.0.2.52", Model: "LX06", Options: map[string]any{"discovery_id": "uuid-study"}},
	}
	known := []discoveredSpeaker{
		{ID: "living", Name: "User Living", IP: "192.0.2.17", Model: "937E4BA6", Options: map[string]any{"keep": "yes"}},
		{ID: "kitchen", Name: "User Kitchen", IP: "192.0.2.51", Model: "LX06-2", Options: map[string]any{"discovery_id": "uuid-kitchen", "custom": true}},
		{ID: "study", Name: "User Study", IP: "192.0.2.15", Model: "LX06", Options: map[string]any{"discovery_id": "uuid-study"}},
	}
	repairs := planDiscoveryRepairs(found, known)
	if len(repairs) != 3 {
		t.Fatalf("repairs=%+v", repairs)
	}
	byID := map[string]discoveryRepair{}
	for _, r := range repairs {
		byID[r.SpeakerID] = r
	}
	if byID["living"].IP != "192.0.2.18" || byID["living"].Options["keep"] != "yes" || byID["living"].Options["discovery_id"] != "mac-living" {
		t.Fatalf("stable migration lost settings: %+v", byID["living"])
	}
	if byID["kitchen"].IP != "192.0.2.116" || byID["kitchen"].Options["discovery_id"] != "uuid-kitchen" || byID["kitchen"].Options["custom"] != true {
		t.Fatalf("kitchen identity migration missing: %+v", byID["kitchen"])
	}
	if byID["study"].IP != "192.0.2.52" || byID["study"].Model != "LX06" {
		t.Fatalf("study identity migration missing: %+v", byID["study"])
	}
}

func TestDiscoveryNeverGuessesByModelOrAddress(t *testing.T) {
	found := []discoveredSpeaker{{ID: "new", Name: "New", IP: "192.0.2.9", Model: "LX06", Options: map[string]any{"discovery_id": "new-device"}}}
	known := []discoveredSpeaker{
		{ID: "old1", Name: "One", IP: "192.0.2.1", Model: "LX06"},
		{ID: "old2", Name: "Two", IP: "192.0.2.2", Model: "LX06"},
	}
	repairs := planDiscoveryRepairs(found, known)
	if len(repairs) != 0 {
		t.Fatalf("identity was guessed from model/address: %+v", repairs)
	}
}

func TestDiscoveryDoesNotRepairAmbiguousStableIdentity(t *testing.T) {
	found := []discoveredSpeaker{{ID: "new", Name: "Kitchen", IP: "192.0.2.9", Options: map[string]any{"discovery_id": "same-id"}}}
	known := []discoveredSpeaker{
		{ID: "one", Name: "One", IP: "192.0.2.1", Options: map[string]any{"discovery_id": "same-id"}},
		{ID: "two", Name: "Two", IP: "192.0.2.2", Options: map[string]any{"discovery_id": "same-id"}},
	}
	if repairs := planDiscoveryRepairs(found, known); len(repairs) != 0 {
		t.Fatalf("ambiguous discovery id was used: %+v", repairs)
	}
}

func TestDiscoveryRepairsReferencedRecordAndPlansUnreferencedDuplicateRemoval(t *testing.T) {
	key := "device-kitchen"
	found := []discoveredSpeaker{{ID: "new-id", Name: "Kitchen", IP: "192.0.2.51", Model: "LX06-2", Options: map[string]any{"discovery_id": key}}}
	known := []discoveredSpeaker{
		{ID: "kitchen-linked", Name: "User Kitchen", IP: "192.0.2.52", Model: "LX06", Options: map[string]any{"discovery_id": key}},
		{ID: "kitchen-duplicate", Name: "Kitchen", IP: "192.0.2.51", Model: "LX06-2", Options: map[string]any{"discovery_id": key}},
	}
	refs := map[string]int{"kitchen-linked": 1}
	repairs := planDiscoveryRepairsWithReferences(found, known, refs)
	if len(repairs) != 1 || repairs[0].SpeakerID != "kitchen-linked" || repairs[0].IP != "192.0.2.51" {
		t.Fatalf("referenced record was not selected for address repair: %+v", repairs)
	}
	deletes := planDuplicateSpeakerDeletes(known, refs)
	if len(deletes) != 1 || deletes[0] != "kitchen-duplicate" {
		t.Fatalf("unreferenced duplicate was not selected: %+v", deletes)
	}
}

func TestDiscoveryPreservesAmbiguousDuplicateRecords(t *testing.T) {
	key := "shared-device-id"
	known := []discoveredSpeaker{
		{ID: "one", Options: map[string]any{"discovery_id": key}},
		{ID: "two", Options: map[string]any{"discovery_id": key}},
	}
	refs := map[string]int{"one": 1, "two": 1}
	if got := planDuplicateSpeakerDeletes(known, refs); len(got) != 0 {
		t.Fatalf("ambiguous referenced records must be preserved: %+v", got)
	}
}

func TestDiscoveryMergesAssociatedNearIdenticalUUIDVariant(t *testing.T) {
	oldKey := "12b06c4a-29b3-4578-af87-d2e06befef03"
	newKey := "12b06c4a-29b3-4578-af87-d2e06befe0f3"
	if !nearUUIDDiscoveryID(oldKey, newKey) {
		t.Fatal("the observed serialization variant should match within two UUID nibbles")
	}
	found := []discoveredSpeaker{{ID: "new-unlinked", Name: "Mi Speaker", IP: "192.0.2.51", Model: "LX06-2", Options: map[string]any{"discovery_id": newKey}}}
	known := []discoveredSpeaker{
		{ID: "kitchen-linked", Name: "User Kitchen", IP: "192.0.2.52", Model: "LX06", Options: map[string]any{"discovery_id": oldKey}},
		{ID: "kitchen-duplicate", Name: "Mi Speaker", IP: "192.0.2.51", Model: "LX06-2", Options: map[string]any{"discovery_id": newKey}},
	}
	refs := map[string]int{"kitchen-linked": 1}
	repairs := planDiscoveryRepairsWithReferences(found, known, refs)
	if len(repairs) != 1 || repairs[0].SpeakerID != "kitchen-linked" || repairs[0].IP != "192.0.2.51" || discoveryKey(repairs[0].Options) != newKey {
		t.Fatalf("associated identity was not migrated: %+v", repairs)
	}
	known[0].IP, known[0].Options = repairs[0].IP, repairs[0].Options
	deletes := planDuplicateSpeakerDeletes(known, refs)
	if len(deletes) != 1 || deletes[0] != "kitchen-duplicate" {
		t.Fatalf("unlinked duplicate was not identified after migration: %+v", deletes)
	}
}

func TestNearUUIDDiscoveryIdentityRejectsMoreThanTwoNibbles(t *testing.T) {
	a := "12b06c4a-29b3-4578-af87-d2e06befef03"
	b := "12b06c4a-29b3-4578-af87-d2e06be00000"
	if nearUUIDDiscoveryID(a, b) {
		t.Fatal("distant UUIDs must not be treated as one speaker")
	}
}

func TestDiscoveryIDConflictDoesNotOverrideKnownStableIdentity(t *testing.T) {
	found := []discoveredSpeaker{{ID: "same", Name: "Other device", IP: "192.0.2.9", Options: map[string]any{"discovery_id": "new-id"}}}
	known := []discoveredSpeaker{{ID: "same", Name: "Existing device", IP: "192.0.2.1", Options: map[string]any{"discovery_id": "old-id"}}}
	if repairs := planDiscoveryRepairs(found, known); len(repairs) != 0 {
		t.Fatalf("conflicting stable identity was migrated: %+v", repairs)
	}
}
