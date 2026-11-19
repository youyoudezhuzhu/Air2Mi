package main

import (
	"strings"
	"testing"
)

func TestAutomaticReceiverIdentityAndFreePort(t *testing.T) {
	known := []receiverRecord{{ID: "previous", Port: 7100}}
	body := map[string]any{"display_name": "书房", "enabled": true, "options": map[string]any{"speaker_id": "speaker"}}
	if err := prepareReceiver(body, known, func(p int) bool { return p != 7101 }); err != nil {
		t.Fatal(err)
	}
	id, _ := body["input_id"].(string)
	if !strings.HasPrefix(id, "ap-") || len(id) != 19 {
		t.Fatal("missing generated identity")
	}
	if body["port"] != 7102 {
		t.Fatalf("port conflict not avoided: %v", body["port"])
	}
	if body["display_name"] != "书房" {
		t.Fatal("name changed")
	}
	if body["kind"] != "airplay" {
		t.Fatal("wrong kind")
	}
}
func TestAutomaticReceiverDoesNotRecycleIdentity(t *testing.T) {
	a := map[string]any{}
	b := map[string]any{}
	free := func(int) bool { return true }
	if err := prepareReceiver(a, nil, free); err != nil {
		t.Fatal(err)
	}
	if err := prepareReceiver(b, []receiverRecord{{ID: a["input_id"].(string), Port: a["port"].(int)}}, free); err != nil {
		t.Fatal(err)
	}
	if a["input_id"] == b["input_id"] || a["port"] == b["port"] {
		t.Fatal("duplicate generated defaults")
	}
}
func TestReceiverPortExhaustionIsAnError(t *testing.T) {
	if prepareReceiver(map[string]any{}, nil, func(int) bool { return false }) == nil {
		t.Fatal("exhaustion hidden")
	}
}
