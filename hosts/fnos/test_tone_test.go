package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestToneAssetIsBoundedDeterministicPCM(t *testing.T) {
	a := tonePCM()
	if !bytes.Equal(a, tonePCM()) {
		t.Fatal("non-deterministic resource")
	}
	for i := 0; i < len(a); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(a[i:]))
		if sample > 4000 || sample < -4000 {
			t.Fatal("tone level too high")
		}
	}
	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := writeToneAsset(path); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	header := make([]byte, 44)
	file.Read(header)
	info, _ := file.Stat()
	if string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" || info.Size() != 44+330*44100*4 {
		t.Fatal("invalid resource")
	}
	sample := make([]byte, len(a))
	file.ReadAt(sample, 44+int64(len(a)))
	if !bytes.Equal(a, sample) {
		t.Fatal("tone pattern not repeated")
	}
}

func TestToneRefusesPlayingAndPausedAirPlayMembership(t *testing.T) {
	for _, state := range []map[string]any{{"river": map[string]any{"producer": "a"}, "instances": map[string]any{}}, {"river": map[string]any{"producer": nil}, "instances": map[string]any{"a": map[string]any{"selected": true, "paused": true}}}, {}} {
		if guardIdle(state) == nil {
			t.Fatal("unsafe or unknown state accepted")
		}
	}
	if err := guardIdle(map[string]any{"river": map[string]any{"producer": nil}, "instances": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
}
func TestToneRejectsBrowserPathsBeforeAnyEngineCall(t *testing.T) {
	m, _ := managerFor(t)
	s := &Server{m: m}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"file":"/etc/passwd"}`))
	w := httptest.NewRecorder()
	s.toneAPI(w, r, "known", false)
	if w.Code != 400 {
		t.Fatalf("path accepted: %d %s", w.Code, w.Body.String())
	}
}
func TestStopRemovesOnlyProvablyDeadEndpoint(t *testing.T) {
	m, _ := managerFor(t)
	path := filepath.Join(m.Data, "engine", "host-data", "endpoint.json")
	if err := writeJSON(path, Endpoint{URL: "http://127.0.0.1:1", Token: "secret", PID: 2147483646, Birth: "123"}); err != nil {
		t.Fatal(err)
	}
	if err := m.stopLocked(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale endpoint retained")
	}
}
func TestFileSessionProfileIsTemporaryAndDoesNotEnableAirPlayOnlyMode(t *testing.T) {
	m, _ := managerFor(t)
	cfg := fileSessionConfig(m)
	if cfg["continuous_miplay_session"] != false || cfg["low_latency_input"] != false || cfg["daily_mode"] != true {
		t.Fatal("file session has incompatible daily AirPlay options")
	}
	if cfg["registry_path"] != filepath.Join(m.Data, "engine", "host-data", "registry.json") {
		t.Fatal("profile outside temporary data")
	}
	if cfg["presentation_delay_ms"] != nil || cfg["start_at_live_edge"] != false {
		t.Fatal("file profile overrides established source-specific startup timing")
	}
	if m.ConfigPath != "" {
		t.Fatal("profile mutated manager")
	}
}

func TestToneManualAndMaximumDeadlineCancelInFlightRequests(t *testing.T) {
	stop := make(chan struct{})
	ctx, cancel := toneRunContext(context.Background(), 15*time.Millisecond, stop)
	defer cancel()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatal(ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("deadline failed")
	}
	manual := make(chan struct{})
	ctx2, cancel2 := toneRunContext(context.Background(), toneMaximumDuration, manual)
	defer cancel2()
	close(manual)
	select {
	case <-ctx2.Done():
		if ctx2.Err() != context.Canceled {
			t.Fatal(ctx2.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("manual stop failed")
	}
	if toneMaximumDuration != 5*time.Minute {
		t.Fatal("maximum is not five minutes")
	}
}
func TestToneOptionsRejectPathsAndInvalidVolumes(t *testing.T) {
	for _, body := range []string{`{"file":"/etc/passwd"}`, `{"volume_percent":0}`, `{"volume_percent":101}`, `{"volume_percent":2.5}`, `{} {}`} {
		if _, err := decodeToneOptions(strings.NewReader(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	value, err := decodeToneOptions(strings.NewReader(`{"volume_percent":25}`))
	if err != nil || value == nil || *value != 25 {
		t.Fatal("valid volume rejected")
	}
}
