package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func bundleFor(t *testing.T, private ed25519.PrivateKey, script string) string {
	t.Helper()
	dir := t.TempDir()
	b := []byte(script)
	digest := sha256.Sum256(b)
	manifest := Manifest{Name: "Air2Mi Engine", KernelVersion: "0.3.11", PackageVersion: "test.1", API: "v1", Platform: "linux", Arch: "x64", Source: strings.Repeat("a", 40), Build: "test", SHA: hex.EncodeToString(digest[:])}
	meta, _ := json.Marshal(manifest)
	for name, data := range map[string][]byte{"engine.run": b, "manifest.json": meta, "manifest.sig": ed25519.Sign(private, meta), "trust.pub": []byte(base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey)))} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func managerFor(t *testing.T) (*Manager, ed25519.PrivateKey) {
	t.Helper()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	bundle := bundleFor(t, key, "#!/bin/sh\nexit 1\n")
	m, err := NewManager(t.TempDir(), bundle, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return m, key
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSignatureAndRestartIntegrity(t *testing.T) {
	m, _ := managerFor(t)
	if err := os.WriteFile(m.active.Binary, []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(m.Data, m.Bundle, m.Bind); err == nil {
		t.Fatal("tampered installed binary trusted on restart")
	}
}

func TestNewManagerAcceptsSymlinkedDataDirectory(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	bundle := bundleFor(t, key, "#!/bin/sh\nexit 1\n")
	root := t.TempDir()
	actual := filepath.Join(root, "persistent-data")
	link := filepath.Join(root, "package-var")
	if err := os.MkdirAll(actual, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(actual, bundle, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(link, bundle, "127.0.0.1"); err != nil {
		t.Fatalf("same data directory through fnOS symlink rejected: %v", err)
	}
}

func TestStopEngineFallbackDoesNotInstallOrRaceHost(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	bundle := bundleFor(t, key, "#!/bin/sh\nexit 1\n")
	data := t.TempDir()
	if err := stopEngineWithoutHost(data, bundle, "127.0.0.1"); err != nil {
		t.Fatalf("empty install should already be stopped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "active-engine.json")); !os.IsNotExist(err) {
		t.Fatal("stop fallback installed an Engine as a side effect")
	}
	if _, err := NewManager(data, bundle, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(data, "host.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if err = stopEngineWithoutHost(data, bundle, "127.0.0.1"); err == nil || !strings.Contains(err.Error(), "生命周期锁") {
		t.Fatalf("fallback did not refuse to race a live Host: %v", err)
	}
}

func TestStopEngineFallbackStopsVerifiedOrphanEngine(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	dataAlias := filepath.Join(root, "package-var")
	if err := os.Symlink(data, dataAlias); err != nil {
		t.Fatal(err)
	}
	hostData := filepath.Join(data, "engine", "host-data")
	if err := os.MkdirAll(hostData, 0700); err != nil {
		t.Fatal(err)
	}
	stopLog := filepath.Join(root, "engine-stop.log")
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	engine := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$BINARY_STOP_LOG\"\nexit 0\n"
	bundle := bundleFor(t, key, engine)
	m, err := NewManager(data, bundle, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BINARY_STOP_LOG", stopLog)
	m.client = &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"api_version":"v1"}`)), Request: r}, nil
	})}
	proc := exec.Command("python3", "-c", "import time; time.sleep(60)", "engine_runtime.desktop", hostData)
	if err = proc.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = proc.Process.Kill()
		_ = proc.Wait()
	}()
	started := time.Now()
	for {
		if _, err = birth(proc.Process.Pid); err == nil {
			break
		}
		if time.Since(started) > 2*time.Second {
			t.Fatalf("orphan test process did not become inspectable: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	created, err := birth(proc.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	ep := Endpoint{URL: "http://127.0.0.1:1", Token: "test-token", PID: proc.Process.Pid, Birth: created}
	if err = writeJSON(filepath.Join(hostData, "endpoint.json"), ep); err != nil {
		t.Fatal(err)
	}
	if err = stopEngineWithoutHost(dataAlias, bundle, m.Bind); err != nil {
		t.Fatalf("verified orphan Engine was not stopped: %v", err)
	}
	called, err := os.ReadFile(stopLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(called), "host stop") {
		t.Fatalf("signed Engine stop command not invoked: %s", called)
	}
}

func TestFnOSStopScriptFallsBackToEngineCleanup(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	bin := filepath.Join(app, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(app, "engine"), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "args.log")
	fakeHost := "#!/bin/sh\ncase \" $* \" in *\" --status \"*) exit 3;; esac\nprintf '%s\\n' \"$*\" >> \"$CALL_LOG\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "air2mi-host"), []byte(fakeHost), 0700); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../packaging/fnos/cmd/main")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, "stop")
	cmd.Env = append(os.Environ(), "TRIM_APPDEST="+app, "TRIM_PKGVAR="+filepath.Join(root, "var"), "CALL_LOG="+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stop script failed: %v: %s", err, out)
	}
	called, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(called), "--stop-engine") || !strings.Contains(string(called), filepath.Join(app, "engine")) {
		t.Fatalf("stop script did not invoke signed Engine fallback: %s", called)
	}
}
func TestWrongSignatureCannotChangeActive(t *testing.T) {
	m, _ := managerFor(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	candidate := bundleFor(t, other, "#!/bin/sh\nexit 2\n")
	old := m.active.Binary
	if err := m.updateFrom(candidate); err == nil {
		t.Fatal("invalid signature accepted")
	}
	if m.active.Binary != old {
		t.Fatal("invalid update changed active")
	}
}
func TestFailedCandidatePreservesUserData(t *testing.T) {
	m, key := managerFor(t)
	root := filepath.Join(m.Data, "engine")
	files := map[string]string{"registry.json": "{\"inputs\":[\"living\"]}", "identities/device.json": "private-identity", "host-data/api-token": "secret", "associations.json": "keep-me"}
	for name, data := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte(data), 0600)
	}
	old := m.active.Binary
	candidate := bundleFor(t, key, "#!/bin/sh\necho 'candidate failure'\nexit 42\n")
	if err := m.updateFrom(candidate); err == nil || !strings.Contains(err.Error(), "回滚") {
		t.Fatalf("expected rollback: %v", err)
	}
	if m.active.Binary != old {
		t.Fatal("old executable not restored")
	}
	for name, data := range files {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(b) != data {
			t.Fatalf("user data changed %s: %v", name, err)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(m.Data, "backups", "*", "identities", "device.json"))
	if len(backups) != 1 {
		t.Fatal("identity backup missing")
	}
}
func TestUpdateZipRejectsTraversalDuplicatesAndMissing(t *testing.T) {
	for _, names := range [][]string{{"../registry.json"}, {"engine.run", "engine.run"}, {"manifest.json"}} {
		p := filepath.Join(t.TempDir(), "u.zip")
		f, _ := os.Create(p)
		z := zip.NewWriter(f)
		for _, name := range names {
			w, _ := z.Create(name)
			w.Write([]byte("data"))
		}
		z.Close()
		f.Close()
		if err := unpackUpdate(p, t.TempDir()); err == nil {
			t.Fatalf("unsafe archive accepted: %v", names)
		}
	}
}
func TestGatewayAdminAndCSRF(t *testing.T) {
	m, _ := managerFor(t)
	s := &Server{m: m, base: "/app/air2mi", csrf: "nonce", www: t.TempDir()}
	cases := []struct {
		trusted            bool
		admin              string
		method, path, csrf string
		want               int
	}{{false, "true", "GET", "/api/state", "", 403}, {true, "false", "GET", "/api/state", "", 403}, {true, "true", "GET", "/api/state", "", 200}, {true, "true", "POST", "/api/lifecycle/stop", "", 403}, {true, "true", "POST", "/api/lifecycle/stop", "nonce", 200}}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, s.base+c.path, nil).WithContext(context.WithValue(context.Background(), peerKey{}, c.trusted))
		r.Header.Set("X-Trim-Isadmin", c.admin)
		r.Header.Set("X-Trim-Userid", "1000")
		r.Header.Set("X-Air2Mi-CSRF", c.csrf)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatalf("%+v got %d: %s", c, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "api-token") || strings.Contains(w.Body.String(), "host-control.token") {
			t.Fatal("private path leaked")
		}
	}
}
func TestMutationOriginAndUnsupportedVolume(t *testing.T) {
	m, _ := managerFor(t)
	s := &Server{m: m, base: "/app/air2mi", csrf: "nonce", devToken: "dev-secret"}
	r := httptest.NewRequest("PATCH", s.base+"/api/engine/v1/speakers/one", strings.NewReader(`{"volume":40}`))
	r.Header.Set("X-Air2Mi-Host-Key", "dev-secret")
	r.Header.Set("X-Air2Mi-CSRF", "nonce")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
	// A stable mDNS device identifier is valid speaker metadata and must reach
	// Engine PATCH; unrelated option keys remain rejected at the Host boundary.
	r = httptest.NewRequest("PATCH", s.base+"/api/engine/v1/speakers/one", strings.NewReader(`{"options":{"discovery_id":"stable-device-uuid"}}`))
	r.Header.Set("X-Air2Mi-Host-Key", "dev-secret")
	r.Header.Set("X-Air2Mi-CSRF", "nonce")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("stable discovery ID rejected by Host: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("PATCH", s.base+"/api/engine/v1/speakers/one", strings.NewReader(`{"options":{"arbitrary":"value"}}`))
	r.Header.Set("X-Air2Mi-Host-Key", "dev-secret")
	r.Header.Set("X-Air2Mi-CSRF", "nonce")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("untrusted metadata accepted: %d", w.Code)
	}
	r = httptest.NewRequest("POST", s.base+"/api/lifecycle/start", nil)
	r.Header.Set("X-Air2Mi-Host-Key", "dev-secret")
	r.Header.Set("X-Air2Mi-CSRF", "nonce")
	r.Header.Set("Origin", "https://evil.invalid")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin accepted")
	}
}
func TestBaseRedirectAndRedaction(t *testing.T) {
	s := &Server{base: "/app/air2mi"}
	r := httptest.NewRequest(http.MethodGet, "/app/air2mi", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 307 || w.Header().Get("Location") != "/app/air2mi/" {
		t.Fatal("prefix redirect")
	}
	value := redact("ordinary\nAuthorization: Bearer xyz\napi_token=secret\nend")
	if bytes.Contains([]byte(value), []byte("secret")) || strings.Contains(value, "xyz") {
		t.Fatal("secret leaked")
	}
}

func TestRestrictedFnOSPath(t *testing.T) {
	p := hostPath("/var/apps/air2mi/target/bin")
	for _, needed := range []string{"/var/apps/air2mi/target/bin", "/usr/bin", "/bin"} {
		found := false
		for _, entry := range strings.Split(p, ":") {
			if entry == needed {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s", needed)
		}
	}
	if hostPath(p) != p {
		t.Fatal("PATH not idempotent")
	}
}

func TestGatewayRewrittenHostOriginRequiresAdminAndNonce(t *testing.T) {
	m, _ := managerFor(t)
	s := &Server{m: m, base: "/app/air2mi", csrf: "nonce"}
	cases := []struct {
		name, origin, csrf, site, admin string
		trusted                         bool
		want                            int
	}{
		{"gateway public HTTP origin", "http://nas.local:5666", "nonce", "same-origin", "true", true, 200},
		{"gateway public HTTPS origin", "https://nas.example", "nonce", "same-origin", "true", true, 200},
		{"gateway missing nonce", "https://nas.example", "", "same-origin", "true", true, 403},
		{"gateway wrong nonce", "https://nas.example", "wrong", "same-origin", "true", true, 403},
		{"untrusted peer", "https://nas.example", "nonce", "same-origin", "true", false, 403},
		{"not admin", "https://nas.example", "nonce", "same-origin", "false", true, 403},
		{"cross-site fetch", "https://attacker.example", "nonce", "cross-site", "true", true, 403},
		{"opaque origin", "null", "nonce", "same-origin", "true", true, 403},
		{"invalid origin with path", "https://nas.example/evil", "nonce", "same-origin", "true", true, 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://localhost/app/air2mi/api/lifecycle/stop", strings.NewReader("{}"))
			r = r.WithContext(context.WithValue(r.Context(), peerKey{}, c.trusted))
			r.Header.Set("X-Trim-Userid", "1000")
			r.Header.Set("X-Trim-Isadmin", c.admin)
			r.Header.Set("X-Air2Mi-CSRF", c.csrf)
			r.Header.Set("Sec-Fetch-Site", c.site)
			r.Header.Set("Origin", c.origin)
			r.Header.Set("X-Forwarded-Host", "nas.example")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("got %d want %d: %s", w.Code, c.want, w.Body.String())
			}
		})
	}
}

func TestEventsRefreshesPlaybackSnapshotWithinHalfSecond(t *testing.T) {
	m, _ := managerFor(t)
	s := &Server{m: m}
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.events(w, r)
		close(done)
	}()
	time.Sleep(1150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("events handler did not stop after client disconnect")
	}
	if got := strings.Count(w.Body.String(), ": keepalive\n\n"); got < 2 {
		t.Fatalf("expected playback snapshots to refresh every 500ms, got %d keepalives", got)
	}
}
func TestDevCannotTrustForwardedOrigin(t *testing.T) {
	s := &Server{devToken: "dev"}
	r := httptest.NewRequest("POST", "http://localhost/test", nil)
	r.Header.Set("Origin", "https://external.example")
	r.Header.Set("X-Forwarded-Host", "external.example")
	if s.validOrigin(r) {
		t.Fatal("forwarded header bypassed origin check")
	}
}

func TestHostWithoutBundledEngineAndFailedFirstInstall(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	bundle := bundleFor(t, key, "#!/bin/sh\nexit 1\n")
	if err := os.Remove(filepath.Join(bundle, "engine.run")); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(t.TempDir(), bundle, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if m.active.Binary != "" || m.snapshot()["engine_installed"] != false {
		t.Fatal("missing Engine treated as installed")
	}
	if err = m.lifecycle("start"); err == nil {
		t.Fatal("started missing Engine")
	}
	candidate := bundleFor(t, key, "#!/bin/sh\nexit 2\n")
	if err = m.updateFrom(candidate); err == nil {
		t.Fatal("failed first install accepted")
	}
	if _, err = NewManager(m.Data, bundle, m.Bind); err != nil {
		t.Fatalf("failed first install left broken pointer: %v", err)
	}
}

func TestLicenseProxyAllowsOnlyEngineActionsAndKeepsCSRF(t *testing.T) {
	m, _ := managerFor(t)
	s := &Server{m: m, base: "/app/air2mi", csrf: "nonce", devToken: "dev-secret"}
	cases := []struct {
		method, path, csrf string
		want               int
	}{
		{"GET", "/api/engine/v1/license", "", 503},
		{"GET", "/api/engine/v1/license/offers", "", 503},
		{"POST", "/api/engine/v1/license/activate", "nonce", 503},
		{"POST", "/api/engine/v1/license/restore", "nonce", 503},
		{"POST", "/api/engine/v1/license/validate", "nonce", 503},
		{"POST", "/api/engine/v1/license/clear", "nonce", 503},
		{"POST", "/api/engine/v1/license/clear", "", 403},
		{"POST", "/api/engine/v1/license/activate", "", 403},
		{"POST", "/api/engine/v1/license", "nonce", 405},
		{"PATCH", "/api/engine/v1/license/activate", "nonce", 405},
		{"GET", "/api/engine/v1/license/activate", "", 405},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, s.base+c.path, strings.NewReader(`{}`))
		r.Header.Set("X-Air2Mi-Host-Key", "dev-secret")
		r.Header.Set("X-Air2Mi-CSRF", c.csrf)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatalf("%s %s got %d want %d: %s", c.method, c.path, w.Code, c.want, w.Body.String())
		}
	}
}
