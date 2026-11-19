package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func signedCatalog(t *testing.T, key ed25519.PrivateKey, catalog EngineCatalog) []byte {
	t.Helper()
	payload, _ := json.Marshal(catalog)
	raw, _ := json.Marshal(map[string]any{"format": "air2mi-update-v1", "key_id": "air2mi-update-v1", "payload": base64.RawURLEncoding.EncodeToString(payload), "signature": base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, payload))})
	return raw
}
func digestBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func TestOnlineCatalogRawSignatureAndClaims(t *testing.T) {
	m, key := managerFor(t)
	catalog := EngineCatalog{Component: "engine", Schema: 1, App: "air2mi", Channel: "stable", Sequence: 7}
	raw := signedCatalog(t, key, catalog)
	if _, e := decodeCatalog(raw, m.public); e != nil {
		t.Fatal(e)
	}
	for _, value := range [][]byte{bytes.Replace(raw, []byte("air2mi-update-v1"), []byte("other-format"), 1), append([]byte("not JSON"), raw...)} {
		if _, e := decodeCatalog(value, m.public); e == nil {
			t.Fatal("bad envelope accepted")
		}
	}
	var envelope map[string]string
	json.Unmarshal(raw, &envelope)
	envelope["payload"] = base64.RawURLEncoding.EncodeToString([]byte(`{"schema_version":1,"app_id":"air2mi","channel":"stable","sequence":99}`))
	tampered, _ := json.Marshal(envelope)
	if _, e := decodeCatalog(tampered, m.public); e == nil {
		t.Fatal("changed signed payload accepted")
	}
	catalog.App = "another-app"
	if _, e := decodeCatalog(signedCatalog(t, key, catalog), m.public); e == nil {
		t.Fatal("wrong product accepted")
	}
}
func TestOnlineUnpublishedDowngradeAndNoArbitraryURL(t *testing.T) {
	m, key := managerFor(t)
	catalog := EngineCatalog{Component: "engine", Schema: 1, App: "air2mi", Channel: "stable", Sequence: 7, Reason: "not published"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(signedCatalog(t, key, catalog)) }))
	defer server.Close()
	u := m.online()
	u.catalogURL = server.URL + "/catalog"
	u.allowTestHTTP = true
	status, e := u.check(context.Background())
	if e != nil || status.Available || status.Phase != "unpublished" {
		t.Fatalf("%+v %v", status, e)
	}
	if e = u.begin("https://attacker.invalid/file"); e == nil {
		t.Fatal("client-controlled release accepted")
	}
	m.active.Manifest.UpdateSequence = 8
	if _, e = u.check(context.Background()); e == nil || !strings.Contains(e.Error(), "回退") {
		t.Fatalf("rollback not rejected: %v", e)
	}
	u.allowTestHTTP = false
	for _, value := range []string{"http://youyoudezhuzhu.github.io/Air2Mi/file", "https://127.0.0.1/file", "https://youyoudezhuzhu.github.io/other/file", "https://user:pass@github.com/youyoudezhuzhu/Air2Mi/releases/download/tag/file"} {
		if u.allowedURL(value) == nil {
			t.Fatal("untrusted URL accepted: " + value)
		}
	}
}
func TestOnlineFallbackAndFailedHealthPreserveData(t *testing.T) {
	m, key := managerFor(t)
	candidate := bundleFor(t, key, "#!/bin/sh\necho failure\nexit 42\n")
	manifestPath := filepath.Join(candidate, "manifest.json")
	raw, _ := os.ReadFile(manifestPath)
	var manifest Manifest
	json.Unmarshal(raw, &manifest)
	manifest.UpdateSequence = 7
	raw, _ = json.Marshal(manifest)
	os.WriteFile(manifestPath, raw, 0600)
	os.WriteFile(filepath.Join(candidate, "manifest.sig"), ed25519.Sign(key, raw), 0600)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"engine.run", "manifest.json", "manifest.sig"} {
		entry, _ := writer.Create(name)
		content, _ := os.ReadFile(filepath.Join(candidate, name))
		entry.Write(content)
	}
	writer.Close()
	archive := buffer.Bytes()
	var catalog EngineCatalog
	var fallback atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("private API authorization leaked to public release source")
		}
		switch r.URL.Path {
		case "/catalog":
			w.Write(signedCatalog(t, key, catalog))
		case "/broken":
			w.Write([]byte("bad"))
		case "/good":
			fallback.Store(true)
			w.Write(archive)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	target := EngineTarget{Platform: "linux", Architecture: "x64", Kind: "engine_zip", URL: server.URL + "/broken", FallbackURL: server.URL + "/good", Size: int64(len(archive)), SHA256: digestBytes(archive), EngineSHA256: manifest.SHA, Source: manifest.Source, PackageVersion: manifest.PackageVersion, KernelVersion: manifest.KernelVersion, API: "v1"}
	catalog = EngineCatalog{Component: "engine", Schema: 1, App: "air2mi", Channel: "stable", Sequence: 7, Published: true, Targets: []EngineTarget{target}}
	marker := filepath.Join(m.Data, "engine/identities/preserve")
	os.MkdirAll(filepath.Dir(marker), 0700)
	os.WriteFile(marker, []byte("identity and association sentinel"), 0600)
	before := m.active.Binary
	u := m.online()
	u.catalogURL = server.URL + "/catalog"
	u.allowTestHTTP = true
	status, e := u.check(context.Background())
	if e != nil || !status.Available {
		t.Fatalf("%+v %v", status, e)
	}
	if e = u.begin(status.ReleaseID); e != nil {
		t.Fatal(e)
	}
	end := time.Now().Add(8 * time.Second)
	for time.Now().Before(end) {
		status = u.snapshot()
		if status.Phase == "error" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.Phase != "error" || !strings.Contains(status.Reason, "回滚") || !fallback.Load() {
		t.Fatalf("fallback/rollback did not finish: %+v", status)
	}
	after, _ := os.ReadFile(marker)
	if m.active.Binary != before || string(after) != "identity and association sentinel" {
		t.Fatal("failed online update damaged installed binary or user data")
	}
}

func TestAppAndEngineUpdatesAreIndependent(t *testing.T) {
	m, key := managerFor(t)
	u := m.appOnline()
	engine := m.online()
	if u == engine || u.catalogURL == engine.catalogURL {
		t.Fatal("channels merged")
	}
	archive := []byte("FPK fixture")
	target := EngineTarget{Platform: "linux", Architecture: "x64", Kind: "fpk", SHA256: digestBytes(archive), Size: int64(len(archive)), AppVersion: "9.0.0", PackageVersion: "9.0.0", Source: strings.Repeat("a", 40), API: "v1"}
	catalog := EngineCatalog{Component: "app", Schema: 1, App: "air2mi", Channel: "stable", Published: true, Sequence: 1}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/catalog" {
			w.Write(signedCatalog(t, key, catalog))
		} else {
			w.Write(archive)
		}
	}))
	defer server.Close()
	target.URL = server.URL + "/file"
	catalog.Targets = []EngineTarget{target}
	u.catalogURL = server.URL + "/catalog"
	u.allowTestHTTP = true
	m.active.Manifest.UpdateSequence = 999
	state, e := u.check(context.Background())
	if e != nil || !state.Available || state.CurrentVersion != version {
		t.Fatalf("app check: %+v %v", state, e)
	}
	if _, e = decodeCatalog(signedCatalog(t, key, catalog), m.public); e == nil {
		t.Fatal("app catalog replayed as engine")
	}
	before := m.active.Binary
	if e = u.install(target, 1); e != nil {
		t.Fatal(e)
	}
	if m.active.Binary != before || engine.snapshot().Phase != "idle" {
		t.Fatal("app download touched engine")
	}
	content, e := os.ReadFile(u.appFile)
	if e != nil || !bytes.Equal(content, archive) {
		t.Fatal("FPK download failed")
	}
	for _, pair := range [][2]string{{"0.1.13", "0.1.12"}, {"0.10.0", "0.9.0"}, {"2.0.0", "1.99.99"}} {
		if !newerAppVersion(pair[0], pair[1]) || newerAppVersion(pair[1], pair[0]) {
			t.Fatal("wrong version order")
		}
	}
	for _, bad := range []string{"1.0", "1.0.0-beta", "../1", "-1.0.0"} {
		if validAppVersion(bad) {
			t.Fatal("bad version accepted")
		}
	}
}
