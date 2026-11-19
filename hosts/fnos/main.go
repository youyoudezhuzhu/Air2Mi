package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var version = "0.1.14"

type Manifest struct {
	Name           string `json:"name"`
	KernelVersion  string `json:"kernel_version"`
	PackageVersion string `json:"package_version"`
	API            string `json:"api_version"`
	Platform       string `json:"platform"`
	Arch           string `json:"architecture"`
	Source         string `json:"source_commit"`
	Build          string `json:"build_id"`
	SHA            string `json:"sha256"`
	UpdateSequence int64  `json:"update_sequence,omitempty"`
}
type Endpoint struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
	Birth string `json:"birth"`
}
type Active struct {
	Binary   string   `json:"binary"`
	Manifest Manifest `json:"manifest"`
}
type Manager struct {
	updatesMu                             sync.Mutex
	updates                               *EngineUpdates
	appUpdates                            *EngineUpdates
	mu                                    sync.Mutex
	Data, Bundle, Bind, Cache, ConfigPath string
	active                                Active
	public                                ed25519.PublicKey
	client                                *http.Client
	operation                             string
	lastError                             string
	runningBind                           string
	tones                                 *ToneController
	cachedConfig                          map[string]any
}

func atomicFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".air2mi-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func writeJSON(path string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	return atomicFile(path, append(b, '\n'), 0600)
}
func checkManifest(binary, manifest, sig string, public ed25519.PublicKey) (Manifest, error) {
	var m Manifest
	b, e := os.ReadFile(manifest)
	if e != nil {
		return m, e
	}
	signature, e := os.ReadFile(sig)
	if e != nil {
		return m, e
	}
	if !ed25519.Verify(public, b, signature) {
		return m, errors.New("Engine 签名校验失败")
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return m, e
	}
	if m.Platform != "linux" || m.Arch != "x64" || m.API != "v1" || m.KernelVersion == "" || len(m.Source) != 40 || m.Build == "" {
		return m, errors.New("Engine 平台/API/版本不兼容")
	}
	f, e := os.Open(binary)
	if e != nil {
		return m, e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return m, e
	}
	if hex.EncodeToString(h.Sum(nil)) != m.SHA {
		return m, errors.New("Engine SHA256 校验失败")
	}
	return m, nil
}
func NewManager(data, bundle, bind string) (*Manager, error) {
	if e := os.MkdirAll(data, 0700); e != nil {
		return nil, e
	}
	pub, e := os.ReadFile(filepath.Join(bundle, "trust.pub"))
	if e != nil {
		return nil, e
	}
	key, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pub)))
	if e != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("缺少可信 Engine 公钥")
	}
	if raw, readErr := os.ReadFile(filepath.Join(data, "network-bind.json")); readErr == nil {
		var saved struct {
			IPv4 string `json:"ipv4"`
		}
		if json.Unmarshal(raw, &saved) == nil && net.ParseIP(saved.IPv4) != nil && net.ParseIP(saved.IPv4).To4() != nil {
			bind = saved.IPv4
		}
	}
	m := &Manager{Data: data, Bundle: bundle, Bind: bind, public: key, client: &http.Client{Timeout: 10 * time.Second}}
	m.tones = &ToneController{owner: m, state: ToneState{Phase: "idle", ChangedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	active := filepath.Join(data, "active-engine.json")
	if b, e := os.ReadFile(active); e == nil {
		if e = json.Unmarshal(b, &m.active); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	} else {
		if _, err := os.Stat(filepath.Join(bundle, "engine.run")); os.IsNotExist(err) {
			return m, nil
		} else if err != nil {
			return nil, err
		}
		a, e := m.installRelease(bundle)
		if e != nil {
			return nil, e
		}
		m.active = a
		if e = writeJSON(active, a); e != nil {
			return nil, e
		}
	}
	folder := filepath.Dir(m.active.Binary)
	expected := filepath.Join(m.Data, "releases", m.active.Manifest.SHA, "engine.run")
	activePath, activeErr := filepath.EvalSymlinks(m.active.Binary)
	expectedPath, expectedErr := filepath.EvalSymlinks(expected)
	if activeErr != nil || expectedErr != nil || filepath.Clean(activePath) != filepath.Clean(expectedPath) {
		return nil, errors.New("非法 Engine 版本指针")
	}
	meta, e := checkManifest(m.active.Binary, filepath.Join(folder, "manifest.json"), filepath.Join(folder, "manifest.sig"), m.public)
	if e != nil {
		return nil, e
	}
	m.active.Manifest = meta
	return m, nil
}
func (m *Manager) installRelease(folder string) (Active, error) {
	var a Active
	binary := filepath.Join(folder, "engine.run")
	meta, e := checkManifest(binary, filepath.Join(folder, "manifest.json"), filepath.Join(folder, "manifest.sig"), m.public)
	if e != nil {
		return a, e
	}
	dest := filepath.Join(m.Data, "releases", meta.SHA)
	if e = os.MkdirAll(dest, 0700); e != nil {
		return a, e
	}
	for _, name := range []string{"engine.run", "manifest.json", "manifest.sig"} {
		b, e := os.ReadFile(filepath.Join(folder, name))
		if e != nil {
			return a, e
		}
		mode := os.FileMode(0600)
		if name == "engine.run" {
			mode = 0700
		}
		if e = atomicFile(filepath.Join(dest, name), b, mode); e != nil {
			return a, e
		}
	}
	return Active{filepath.Join(dest, "engine.run"), meta}, nil
}
func birth(pid int) (string, error) {
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return "", e
	}
	s := strings.Fields(string(b)[strings.LastIndex(string(b), ")")+1:])
	if len(s) < 20 {
		return "", errors.New("PID 元数据无效")
	}
	if s[0] == "Z" {
		return "", os.ErrProcessDone
	}
	return s[19], nil
}
func (m *Manager) endpoint() (Endpoint, error) {
	var ep Endpoint
	b, e := os.ReadFile(filepath.Join(m.Data, "engine", "host-data", "endpoint.json"))
	if e != nil {
		return ep, e
	}
	if e = json.Unmarshal(b, &ep); e != nil {
		return ep, e
	}
	u, e := url.Parse(ep.URL)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || ep.Token == "" || ep.PID < 2 {
		return ep, errors.New("Engine 私有端点无效")
	}
	actual, e := birth(ep.PID)
	if e != nil || actual != ep.Birth {
		return ep, errors.New("Engine PID 已退出或被复用")
	}
	args, e := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", ep.PID))
	hostData, pathErr := filepath.EvalSymlinks(filepath.Join(m.Data, "engine", "host-data"))
	if e != nil || pathErr != nil || !strings.Contains(string(args), "engine_runtime.desktop") || !strings.Contains(string(args), hostData) {
		return ep, errors.New("拒绝管理不属于本应用的 Engine")
	}
	return ep, nil
}
func (m *Manager) request(path, method string, body io.Reader) (*http.Response, error) {
	return m.requestContext(context.Background(), path, method, body)
}
func (m *Manager) requestContext(ctx context.Context, path, method string, body io.Reader) (*http.Response, error) {
	ep, e := m.endpoint()
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, method, ep.URL+path, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	req.Header.Set("Content-Type", "application/json")
	return m.client.Do(req)
}
func (m *Manager) get(path string) (any, error) { return m.getContext(context.Background(), path) }
func (m *Manager) getContext(ctx context.Context, path string) (any, error) {
	resp, e := m.requestContext(ctx, path, "GET", nil)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Engine API %s: HTTP %d", path, resp.StatusCode)
	}
	var out any
	e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out)
	return out, e
}
func (m *Manager) command(ctx context.Context, args ...string) ([]byte, error) {
	if m.active.Binary == "" {
		return []byte("尚未安装 Engine，请在设置中安装本地签名内核组件"), errors.New("engine_not_installed")
	}
	if m.ConfigPath != "" && len(args) >= 2 && args[0] == "host" && args[1] == "start" {
		args = append(args, "--config", m.ConfigPath)
	}
	cmd := exec.CommandContext(ctx, m.active.Binary, append([]string{"--data-dir", filepath.Join(m.Data, "engine")}, args...)...)
	cmd.Env = append(os.Environ(), "A2M_CACHE_DIR="+m.cacheDir())
	out, e := cmd.CombinedOutput()
	if len(out) > 128<<10 {
		out = out[len(out)-(128<<10):]
	}
	return out, e
}
func (m *Manager) startLocked() error {
	if m.active.Binary == "" {
		return errors.New("尚未安装 Engine，请在设置中安装本地签名内核组件")
	}
	if _, e := m.endpoint(); e == nil {
		m.runningBind = m.Bind
		return nil
	}
	epPath := filepath.Join(m.Data, "engine", "host-data", "endpoint.json")
	if b, e := os.ReadFile(epPath); e == nil {
		var ep Endpoint
		if json.Unmarshal(b, &ep) != nil {
			return errors.New("私有端点损坏，拒绝猜测进程身份")
		}
		if actual, e := birth(ep.PID); e == nil && actual == ep.Birth {
			return errors.New("端点属于未知运行进程，拒绝覆盖")
		}
		if e = os.Remove(epPath); e != nil {
			return e
		}
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 35*time.Second)
	_, selectedBind, probeErr := m.probeDiscovery(probeCtx)
	probeCancel()
	if probeErr != nil {
		return probeErr
	}
	m.Bind = selectedBind
	if err := writeJSON(filepath.Join(m.Data, "network-bind.json"), map[string]string{"ipv4": selectedBind, "mode": "auto-discovered"}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	out, e := m.command(ctx, "host", "start", "--bind", m.Bind)
	if e != nil {
		return fmt.Errorf("Engine 启动失败: %s", redact(string(out)))
	}
	health, e := m.get("/v1/health")
	if e != nil {
		return e
	}
	h, _ := health.(map[string]any)
	if h["api_version"] != "v1" {
		return errors.New("Engine API 不兼容")
	}
	m.runningBind = m.Bind
	return nil
}
func (m *Manager) stopLocked() error {
	if _, e := m.endpoint(); e != nil {
		path := filepath.Join(m.Data, "engine", "host-data", "endpoint.json")
		raw, readErr := os.ReadFile(path)
		if os.IsNotExist(readErr) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		var stale Endpoint
		if json.Unmarshal(raw, &stale) != nil {
			return e
		}
		actual, birthErr := birth(stale.PID)
		if os.IsNotExist(birthErr) || errors.Is(birthErr, os.ErrProcessDone) || (birthErr == nil && actual != stale.Birth) {
			return os.Remove(path)
		}
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	out, e := m.command(ctx, "host", "stop")
	if e != nil {
		return fmt.Errorf("Engine 停止失败: %s", redact(string(out)))
	}
	m.runningBind = ""
	return nil
}

// stopEngineWithoutHost is the fnOS lifecycle fallback used when the Go Host
// itself is unavailable. The lock prevents racing a live Host, while
// NewManager verifies the signed active Engine before sending it a stop
// command. It never guesses a PID or executes a path from an unverified file.
func stopEngineWithoutHost(data, bundle, bind string) error {
	activePath := filepath.Join(data, "active-engine.json")
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		endpointPath := filepath.Join(data, "engine", "host-data", "endpoint.json")
		if _, endpointErr := os.Stat(endpointPath); os.IsNotExist(endpointErr) {
			return nil
		}
		return errors.New("Engine 端点存在但版本指针缺失，拒绝猜测并停止进程")
	} else if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(data, "host.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("管理 Host 仍持有生命周期锁，拒绝并发停止 Engine")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	m, err := NewManager(data, bundle, bind)
	if err != nil {
		return err
	}
	return m.lifecycle("stop")
}

func (m *Manager) lifecycle(action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toneBusy() {
		return errors.New("请先停止测试音，再操作内核")
	}
	m.operation = action
	defer func() { m.operation = "" }()
	var e error
	switch action {
	case "start":
		e = m.startLocked()
	case "stop":
		e = m.stopLocked()
	case "restart":
		if e = m.stopLocked(); e == nil {
			e = m.startLocked()
		}
	default:
		e = errors.New("未知生命周期操作")
	}
	if e != nil {
		m.lastError = e.Error()
	} else {
		m.lastError = ""
	}
	return e
}
func redact(s string) string {
	for _, line := range strings.Split(s, "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "authorization") || strings.Contains(low, "token") || strings.Contains(low, "private_key") || strings.Contains(low, "aes_key") {
			s = strings.ReplaceAll(s, line, "[敏感字段已隐藏]")
		}
	}
	return displayDeviceText(s)
}
func (m *Manager) snapshot() map[string]any {
	m.mu.Lock()
	a := m.active
	last := m.lastError
	op := m.operation
	cached := map[string]any{}
	for key, value := range m.cachedConfig {
		cached[key] = value
	}
	m.mu.Unlock()
	s := map[string]any{"app_version": version, "environment": environment(), "bind": m.Bind, "engine_manifest": a.Manifest, "engine_running": false, "engine_installed": a.Binary != "", "operation": op, "test_tone": m.toneState(), "last_error": last, "capabilities": map[string]any{"registry": true, "single_target_routes": true, "one_to_one_associations": true, "multi_target_routes": false, "live_speaker_volume": false, "actual_output_state": false, "discovery_preview": true, "diagnostics": true, "online_updates": true, "signed_offline_update": true}, "updates": m.online().snapshot(), "app_updates": m.appOnline().snapshot()}
	if _, e := m.endpoint(); e != nil {
		for key, value := range cached {
			s[key] = value
		}
		s["configuration_cached"] = len(cached) > 0
		if m.toneBusy() {
			s["operation"] = "test_tone"
		} else if a.Binary != "" {
			s["engine_error"] = e.Error()
		}
		return s
	}
	s["engine_running"] = true
	for _, entry := range [][2]string{{"health", "/v1/health"}, {"state", "/v1/state"}, {"config", "/v1/config"}, {"inputs", "/v1/inputs"}, {"speakers", "/v1/speakers"}, {"routes", "/v1/routes"}} {
		v, e := m.get(entry[1])
		if e != nil {
			s[entry[0]+"_error"] = e.Error()
		} else {
			s[entry[0]] = v
		}
	}
	m.mu.Lock()
	if m.cachedConfig == nil {
		m.cachedConfig = map[string]any{}
	}
	for _, key := range []string{"inputs", "speakers", "routes"} {
		if value, ok := s[key]; ok {
			m.cachedConfig[key] = value
		}
	}
	m.mu.Unlock()
	if state, ok := s["state"].(map[string]any); ok {
		if available, ok := state["output_status_available"].(bool); ok {
			s["capabilities"].(map[string]any)["actual_output_state"] = available
		}
	}
	return s
}
func (m *Manager) doctor() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	out, e := m.command(ctx, "doctor")
	return redact(string(out)), e
}
func (m *Manager) discovery() (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	speakers, ip, err := m.probeDiscovery(ctx)
	if err != nil {
		return nil, err
	}
	if ip != m.Bind {
		m.Bind = ip
		if err := writeJSON(filepath.Join(m.Data, "network-bind.json"), map[string]string{"ipv4": ip, "mode": "auto-discovered"}); err != nil {
			return nil, err
		}
	}
	return speakers, nil
}

func (m *Manager) probeDiscovery(ctx context.Context) ([]any, string, error) {
	candidates := bindCandidates()
	if len(candidates) == 0 {
		candidates = []string{m.Bind}
	}
	// Try the current address first, then every eligible multicast interface.
	ordered := []string{m.Bind}
	for _, ip := range candidates {
		if ip != m.Bind {
			ordered = append(ordered, ip)
		}
	}
	var lastOutput string
	for _, ip := range ordered {
		preview := filepath.Join(m.Data, "discovery-preview", ip)
		cmd := exec.CommandContext(ctx, m.active.Binary, "--data-dir", preview, "daily", "--prepare-only", "--bind", ip, "--discovery-timeout", "2.5")
		cmd.Env = append(os.Environ(), "A2M_CACHE_DIR="+m.cacheDir())
		out, err := cmd.CombinedOutput()
		if err != nil {
			lastOutput = redact(string(out))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(preview, "engine", "local", "daily", "registry.json"))
		if err != nil {
			lastOutput = err.Error()
			continue
		}
		var registry map[string]any
		if err = json.Unmarshal(data, &registry); err != nil {
			lastOutput = err.Error()
			continue
		}
		speakers, _ := registry["speakers"].([]any)
		if len(speakers) == 0 {
			continue
		}
		return speakers, ip, nil
	}
	if ctx.Err() != nil {
		return nil, "", fmt.Errorf("网卡扫描超时；已尝试 %d 个局域网接口", len(ordered))
	}
	if lastOutput != "" {
		return nil, "", fmt.Errorf("未能在可用网卡上发现妙播音箱。已尝试 %d 个接口；最后错误：%s", len(ordered), lastOutput)
	}
	return nil, "", fmt.Errorf("未发现妙播音箱。已检查 %d 个可用网卡；请确认 NAS 与音箱在同一局域网且允许 mDNS 组播", len(ordered))
}

type Server struct {
	m                                  *Manager
	www, base, devToken, control, csrf string
	shutdown                           chan struct{}
}

const eventSnapshotInterval = 500 * time.Millisecond

func jsonOut(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	raw, err := json.Marshal(value)
	if err == nil {
		_, _ = w.Write(displayDeviceJSON(raw))
	}
}
func (s *Server) authorized(r *http.Request) bool {
	if s.devToken != "" {
		return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Air2Mi-Host-Key")), []byte(s.devToken)) == 1
	}
	trusted, _ := r.Context().Value(peerKey{}).(bool)
	return trusted && r.Header.Get("X-Trim-Isadmin") == "true" && r.Header.Get("X-Trim-Userid") != ""
}

type peerKey struct{}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'")
	path := r.URL.Path
	if path == s.base {
		http.Redirect(w, r, s.base+"/", 307)
		return
	}
	if !strings.HasPrefix(path, s.base+"/") {
		http.NotFound(w, r)
		return
	}
	path = strings.TrimPrefix(path, s.base)
	if strings.HasPrefix(path, "/internal/") {
		trusted, _ := r.Context().Value(peerKey{}).(bool)
		if !trusted || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Air2Mi-Control")), []byte(s.control)) != 1 {
			jsonOut(w, 403, map[string]any{"error": "forbidden"})
			return
		}
		if path == "/internal/health" {
			jsonOut(w, 200, map[string]any{"running": true, "version": version})
			return
		}
		if path == "/internal/stop" && r.Method == "POST" {
			select {
			case s.shutdown <- struct{}{}:
			default:
			}
			jsonOut(w, 200, map[string]any{"stopping": true})
			return
		}
		http.NotFound(w, r)
		return
	}
	if !s.authorized(r) && !(s.devToken != "" && (r.Method == "GET" || r.Method == "HEAD") && !strings.HasPrefix(path, "/api/")) {
		jsonOut(w, 403, map[string]any{"error": "fnOS 管理员登录态验证失败"})
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if !s.validOrigin(r) {
			jsonOut(w, 403, map[string]any{"error": "cross_origin_rejected"})
			return
		}
		if r.Header.Get("X-Air2Mi-CSRF") != s.csrf {
			jsonOut(w, 403, map[string]any{"error": "csrf_required"})
			return
		}
	}
	switch {
	case path == "/api/session":
		jsonOut(w, 200, map[string]any{"csrf": s.csrf, "app_version": version})
	case path == "/api/state":
		jsonOut(w, 200, s.m.snapshot())
	case path == "/api/events":
		s.events(w, r)
	case strings.HasPrefix(path, "/api/lifecycle/") && r.Method == "POST":
		e := s.m.lifecycle(strings.TrimPrefix(path, "/api/lifecycle/"))
		if e != nil {
			jsonOut(w, 409, map[string]any{"error": e.Error()})
		} else {
			jsonOut(w, 200, s.m.snapshot())
		}
	case path == "/api/doctor" && r.Method == "POST":
		out, e := s.m.doctor()
		status := 200
		if e != nil {
			status = 422
		}
		jsonOut(w, status, map[string]any{"output": out, "passed": e == nil})
	case path == "/api/discovery" && r.Method == "POST":
		v, e := s.m.discoverAndAdd()
		if e != nil {
			jsonOut(w, 422, map[string]any{"error": e.Error()})
		} else {
			jsonOut(w, 200, v)
		}
	case strings.HasPrefix(path, "/api/speakers/") && strings.HasSuffix(path, "/test-tone") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/speakers/"), "/test-tone")
		s.toneAPI(w, r, id, false)
	case path == "/api/test-tone" && r.Method == "GET":
		jsonOut(w, 200, s.m.toneState())
	case path == "/api/test-tone/stop" && r.Method == "POST":
		s.toneAPI(w, r, "", true)
	case path == "/api/plus/offers" && r.Method == "GET":
		status, value := publicPlusOffers.get(r.Context())
		jsonOut(w, status, value)
	case path == "/api/app-updates" && r.Method == "GET":
		s.onlineUpdate(w, r)
	case path == "/api/app-update-status" && r.Method == "GET":
		jsonOut(w, 200, s.m.appOnline().snapshot())
	case path == "/api/app-update-online" && r.Method == "POST":
		s.onlineUpdate(w, r)
	case path == "/api/app-update-file" && r.Method == "GET":
		s.appUpdateFile(w, r)
	case path == "/api/updates" && r.Method == "GET":
		s.onlineUpdate(w, r)
	case path == "/api/update-status" && r.Method == "GET":
		jsonOut(w, 200, s.m.online().snapshot())
	case path == "/api/update-online" && r.Method == "POST":
		s.onlineUpdate(w, r)
	case path == "/api/update" && r.Method == "POST":
		s.update(w, r)
	case path == "/api/logs":
		jsonOut(w, 200, map[string]any{"text": s.logs(r.URL.Query().Get("q"), r.URL.Query().Get("level"))})
	case path == "/api/diagnostics":
		w.Header().Set("Content-Disposition", "attachment; filename=air2mi-diagnostics.json")
		jsonOut(w, 200, map[string]any{"snapshot": s.m.snapshot(), "logs": s.logs("", "")})
	case strings.HasPrefix(path, "/api/engine/"):
		s.proxy(w, r, "/"+strings.TrimPrefix(path, "/api/engine/"))
	default:
		if r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		u := *r.URL
		r2.URL = &u
		r2.URL.Path = path
		http.FileServer(http.Dir(s.www)).ServeHTTP(w, r2)
	}
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-cache")
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	// Output playback state is part of the live River topology. A 2-second
	// polling interval made the UI animation lag behind first-audio reports by
	// up to 2 seconds, obscuring startup-latency measurements.
	tick := time.NewTicker(eventSnapshotInterval)
	defer tick.Stop()
	var previous string
	for {
		b, e := json.Marshal(s.m.snapshot())
		if e != nil {
			return
		}
		b = displayDeviceJSON(b)
		if string(b) != previous {
			fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", b)
			previous = string(b)
		} else {
			fmt.Fprint(w, ": keepalive\n\n")
		}
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Server) logs(query, level string) string {
	paths := []string{filepath.Join(s.m.Data, "app.log"), filepath.Join(s.m.Data, "engine", "host-data", "host.log")}
	var text strings.Builder
	for _, p := range paths {
		f, e := os.Open(p)
		if e != nil {
			continue
		}
		info, e := f.Stat()
		if e == nil && info.Size() > 128<<10 {
			_, _ = f.Seek(-(128 << 10), io.SeekEnd)
		}
		b, _ := io.ReadAll(io.LimitReader(f, 128<<10))
		f.Close()
		text.Write(b)
	}
	var lines []string
	for _, line := range strings.Split(redact(text.String()), "\n") {
		if query != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
			continue
		}
		if level != "" && level != "all" && !strings.Contains(strings.ToLower(line), strings.ToLower(level)) {
			continue
		}
		lines = append(lines, strings.ReplaceAll(line, s.m.Data, "<data>"))
	}
	return strings.Join(lines, "\n")
}
func (s *Server) proxy(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "v1" {
		http.NotFound(w, r)
		return
	}
	allowed := map[string]bool{"health": true, "state": true, "config": true, "river": true, "events": true, "inputs": true, "speakers": true, "routes": true, "license": true}
	if !allowed[parts[1]] || len(parts) > 3 {
		http.NotFound(w, r)
		return
	}
	licenseRequest := parts[1] == "license"
	if licenseRequest {
		validGet := r.Method == "GET" && (len(parts) == 2 || (len(parts) == 3 && parts[2] == "offers"))
		validPost := r.Method == "POST" && len(parts) == 3 && (parts[2] == "activate" || parts[2] == "restore" || parts[2] == "validate" || parts[2] == "clear")
		if !validGet && !validPost {
			jsonOut(w, 405, map[string]any{"error": "method_not_allowed"})
			return
		}
		if validPost {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
		}
	}
	if r.Method != "GET" && !licenseRequest {
		if parts[1] != "inputs" && parts[1] != "speakers" && parts[1] != "routes" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "POST" && r.Method != "PATCH" && r.Method != "DELETE" {
			jsonOut(w, 405, map[string]any{"error": "method_not_allowed"})
			return
		}
		s.m.mu.Lock()
		defer s.m.mu.Unlock()
		if s.m.toneBusy() {
			jsonOut(w, 423, map[string]any{"error": "测试音运行中，请先停止测试音"})
			return
		}
		if parts[1] == "speakers" && len(parts) == 3 && r.Method == "DELETE" {
			raw, err := s.m.get("/v1/inputs")
			if err != nil {
				jsonOut(w, 503, map[string]any{"error": err.Error()})
				return
			}
			b, _ := json.Marshal(raw)
			var cfg struct {
				Inputs []struct {
					Name    string         `json:"display_name"`
					Options map[string]any `json:"options"`
				} `json:"inputs"`
			}
			if err = json.Unmarshal(b, &cfg); err != nil {
				jsonOut(w, 502, map[string]any{"error": "invalid_input_registry"})
				return
			}
			for _, input := range cfg.Inputs {
				if input.Options["speaker_id"] == parts[2] {
					jsonOut(w, 409, map[string]any{"error": "该音箱仍关联接收器「" + input.Name + "」，请先修改或删除接收器关联"})
					return
				}
			}
		}
		if r.Method != "DELETE" {
			b, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if e != nil || len(b) > 1<<20 {
				jsonOut(w, 400, map[string]any{"error": "body_too_large"})
				return
			}
			var body map[string]any
			if json.Unmarshal(b, &body) != nil {
				jsonOut(w, 400, map[string]any{"error": "invalid_json"})
				return
			}
			if _, ok := body["identity_file"]; ok {
				jsonOut(w, 400, map[string]any{"error": "identity_path_is_engine_owned"})
				return
			}
			if kind, ok := body["kind"]; ok && kind != "airplay" {
				jsonOut(w, 422, map[string]any{"error": "本客户端仅管理 AirPlay 输入"})
				return
			}
			if parts[1] == "speakers" && len(parts) == 2 && r.Method == "POST" {
				if id, _ := body["speaker_id"].(string); strings.TrimSpace(id) == "" {
					body["speaker_id"] = "sp-" + randomToken()[:16]
					b, e = json.Marshal(body)
					if e != nil {
						jsonOut(w, 400, map[string]any{"error": "invalid_speaker"})
						return
					}
				}
			}
			if parts[1] == "speakers" {
				if raw, ok := body["ip"]; ok {
					ip, valid := raw.(string)
					if !valid || net.ParseIP(ip) == nil || net.ParseIP(ip).To4() == nil {
						jsonOut(w, 422, map[string]any{"error": "请输入有效的 IPv4 地址"})
						return
					}
				}
				if _, ok := body["volume"]; ok {
					jsonOut(w, 422, map[string]any{"error": "Engine 尚未提供独立实时音量 API"})
					return
				}
			}
			if ids, ok := body["speaker_ids"].([]any); ok && len(ids) != 1 {
				jsonOut(w, 422, map[string]any{"error": "Engine 数据面尚未支持此多目标配置"})
				return
			}
			if options, ok := body["options"].(map[string]any); ok {
				for k := range options {
					allowed := (parts[1] == "inputs" && (k == "speaker_id" || k == "device_id")) ||
						(parts[1] == "speakers" && k == "discovery_id") ||
						(parts[1] == "routes" && (k == "speaker_id" || k == "device_id"))
					if !allowed {
						jsonOut(w, 400, map[string]any{"error": "unsupported_registry_option"})
						return
					}
				}
			}
			if parts[1] == "inputs" && len(parts) == 2 && r.Method == "POST" {
				if err := s.m.receiverDefaults(body); err != nil {
					jsonOut(w, 422, map[string]any{"error": err.Error()})
					return
				}
				b, e = json.Marshal(body)
				if e != nil {
					jsonOut(w, 400, map[string]any{"error": "invalid_receiver"})
					return
				}
			}
			if parts[1] == "inputs" || parts[1] == "routes" {
				id := ""
				if len(parts) == 3 {
					id = parts[2]
				}
				if err := s.m.validateAssociationMutation(parts[1], r.Method, id, body); err != nil {
					jsonOut(w, 409, map[string]any{"error": err.Error()})
					return
				}
			}
			r.Body = io.NopCloser(strings.NewReader(string(b)))
			r.ContentLength = int64(len(b))
		}
	}
	ep, e := s.m.endpoint()
	if e != nil {
		jsonOut(w, 503, map[string]any{"error": "engine_not_running", "detail": e.Error()})
		return
	}
	u, _ := url.Parse(ep.URL)
	proxy := httputil.NewSingleHostReverseProxy(u)
	original := proxy.Director
	proxy.Director = func(req *http.Request) {
		original(req)
		req.URL.Path = path
		req.Header.Del("X-Air2Mi-Host-Key")
		req.Header.Del("Cookie")
		req.Header.Del("X-Trim-Userid")
		req.Header.Del("X-Trim-Isadmin")
		req.Header.Del("Accept-Encoding")
		req.Header.Set("Authorization", "Bearer "+ep.Token)
	}
	proxy.FlushInterval = -1
	proxy.ModifyResponse = displayDeviceResponse
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		jsonOut(w, 502, map[string]any{"error": "engine_connection_failed"})
	}
	proxy.ServeHTTP(w, r)
}
func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func trustedPeer(c net.Conn) bool {
	u, ok := c.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, e := u.SyscallConn()
	if e != nil {
		return false
	}
	trusted := false
	_ = raw.Control(func(fd uintptr) {
		cred, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if e == nil {
			trusted = cred.Uid == 0 || int(cred.Uid) == os.Geteuid()
		}
	})
	return trusted
}
func main() {
	// fnOS supplies a restricted package PATH. Native shell wrappers require the
	// system shell utilities; add standard paths without depending on user setup.
	os.Setenv("PATH", hostPath(os.Getenv("PATH")))

	data := flag.String("data", "", "persistent data directory")
	bundle := flag.String("bundle", "", "signed bundled Engine directory")
	www := flag.String("web", "", "Vue dist")
	sock := flag.String("socket", "", "fnOS gateway Unix socket")
	listen := flag.String("listen", "", "explicit loopback development listener")
	dev := flag.String("dev-key", "", "development-only key")
	bind := flag.String("bind", defaultBind(), "Engine bind IPv4")
	stop := flag.Bool("stop", false, "stop owned service")
	status := flag.Bool("status", false, "owned service status")
	stopEngine := flag.Bool("stop-engine", false, "stop the verified Engine when the Host is unavailable")
	flag.Parse()
	if *data == "" {
		fmt.Fprintln(os.Stderr, "--data required")
		os.Exit(2)
	}
	if *stopEngine {
		if *bundle == "" {
			fmt.Fprintln(os.Stderr, "--bundle required with --stop-engine")
			os.Exit(2)
		}
		if e := stopEngineWithoutHost(*data, *bundle, *bind); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	if *sock == "" {
		fmt.Fprintln(os.Stderr, "--socket required")
		os.Exit(2)
	}
	if *stop || *status {
		code := controlCLI(*data, *sock, *stop)
		os.Exit(code)
	}
	if runtime.GOARCH != "amd64" {
		fmt.Fprintln(os.Stderr, "fnOS ARM64 尚未验收")
		os.Exit(1)
	}
	if ip := net.ParseIP(*bind); ip == nil || ip.To4() == nil {
		fmt.Fprintln(os.Stderr, "invalid Engine bind")
		os.Exit(2)
	}
	if e := os.MkdirAll(*data, 0700); e != nil {
		panic(e)
	}
	lock, e := os.OpenFile(filepath.Join(*data, "host.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		panic(e)
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		fmt.Fprintln(os.Stderr, "Host already running")
		os.Exit(1)
	}
	m, e := NewManager(*data, *bundle, *bind)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	secret := randomToken()
	if e = atomicFile(filepath.Join(*data, "host-control.token"), []byte(secret), 0600); e != nil {
		panic(e)
	}
	s := &Server{m: m, www: *www, base: "/app/air2mi", devToken: *dev, control: secret, csrf: randomToken(), shutdown: make(chan struct{}, 1)}
	if info, e := os.Lstat(*sock); e == nil {
		if info.Mode()&os.ModeSocket == 0 {
			panic("socket path occupied by non-socket")
		}
		_ = os.Remove(*sock)
	}
	listener, e := net.Listen("unix", *sock)
	if e != nil {
		panic(e)
	}
	defer os.Remove(*sock)
	_ = os.Chmod(*sock, 0666)
	ctx, cancel := context.WithCancel(context.Background())
	go boundLogs(ctx, filepath.Join(*data, "app.log"), filepath.Join(*data, "engine", "host-data", "host.log"))
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }, ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, peerKey{}, trustedPeer(c))
	}}
	go server.Serve(listener)
	if *listen != "" {
		host, _, e := net.SplitHostPort(*listen)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || *dev == "" {
			panic("development HTTP requires loopback and explicit key")
		}
		go serveTCP(server, *listen)
	}
	_ = writeJSON(filepath.Join(*data, "host-pid.json"), map[string]any{"pid": os.Getpid(), "version": version})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	for {
		select {
		case <-s.shutdown:
		case <-signals:
		}
		if m.tones != nil {
			if stopErr := m.tones.stop(); stopErr != nil {
				fmt.Fprintln(os.Stderr, stopErr)
				continue
			}
		}
		if e = m.lifecycle("stop"); e != nil {
			fmt.Fprintln(os.Stderr, e)
			continue
		}
		break
	}
	cancel()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_ = server.Shutdown(ctx2)
	_ = os.Remove(filepath.Join(*data, "host-pid.json"))
}

// Kept as a function so the production server never silently opens a LAN port.
func serveTCP(server *http.Server, address string) {
	listener, e := net.Listen("tcp", address)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return
	}
	_ = server.Serve(listener)
}
func controlCLI(data, sock string, stop bool) int {
	b, e := os.ReadFile(filepath.Join(data, "host-control.token"))
	if e != nil {
		return 3
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	path := "/internal/health"
	method := "GET"
	if stop {
		path = "/internal/stop"
		method = "POST"
	}
	req, _ := http.NewRequest(method, "http://localhost/app/air2mi"+path, nil)
	req.Header.Set("X-Air2Mi-Control", string(b))
	resp, e := client.Do(req)
	if e != nil {
		return 3
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 1
	}
	if stop {
		for i := 0; i < 250; i++ {
			time.Sleep(200 * time.Millisecond)
			c, e := net.DialTimeout("unix", sock, 200*time.Millisecond)
			if e != nil {
				return 0
			}
			c.Close()
		}
		return 1
	}
	return 0
}
