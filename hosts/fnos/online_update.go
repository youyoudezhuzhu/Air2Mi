package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const engineCatalogURL = "https://youyoudezhuzhu.github.io/Air2Mi/updates/engine-stable.json"

const appCatalogURL = "https://youyoudezhuzhu.github.io/Air2Mi/updates/app-stable.json"

type EngineTarget struct {
	AppVersion     string `json:"app_version"`
	Platform       string `json:"platform"`
	Architecture   string `json:"architecture"`
	Kind           string `json:"kind"`
	URL            string `json:"url"`
	FallbackURL    string `json:"url_alt"`
	SHA256         string `json:"sha256"`
	EngineSHA256   string `json:"engine_sha256"`
	Size           int64  `json:"size"`
	Source         string `json:"source_commit"`
	PackageVersion string `json:"package_version"`
	KernelVersion  string `json:"kernel_version"`
	API            string `json:"api_version"`
}
type EngineCatalog struct {
	Component string         `json:"component"`
	Schema    int            `json:"schema_version"`
	App       string         `json:"app_id"`
	Channel   string         `json:"channel"`
	Published bool           `json:"published"`
	Sequence  int64          `json:"sequence"`
	Reason    string         `json:"reason"`
	Targets   []EngineTarget `json:"targets"`
}
type EngineUpdateStatus struct {
	Component      string `json:"component"`
	CurrentVersion string `json:"current_version"`
	AppVersion     string `json:"app_version,omitempty"`
	Configured     bool   `json:"configured"`
	Phase          string `json:"phase"`
	Available      bool   `json:"available"`
	Reason         string `json:"reason"`
	ReleaseID      string `json:"release_id"`
	PackageVersion string `json:"package_version"`
	KernelVersion  string `json:"kernel_version"`
	Kind           string `json:"kind"`
	Downloaded     int64  `json:"downloaded"`
	Total          int64  `json:"total"`
}
type EngineUpdates struct {
	component     string
	appFile       string
	mu            sync.Mutex
	owner         *Manager
	client        *http.Client
	catalogURL    string
	allowTestHTTP bool
	status        EngineUpdateStatus
	target        EngineTarget
	sequence      int64
	working       bool
}

func (m *Manager) online() *EngineUpdates    { return m.onlineFor("engine") }
func (m *Manager) appOnline() *EngineUpdates { return m.onlineFor("app") }
func (m *Manager) onlineFor(component string) *EngineUpdates {
	m.updatesMu.Lock()
	defer m.updatesMu.Unlock()
	slot := &m.updates
	source := engineCatalogURL
	if component == "app" {
		slot = &m.appUpdates
		source = appCatalogURL
	}
	if *slot == nil {
		u := &EngineUpdates{owner: m, component: component, catalogURL: source, status: EngineUpdateStatus{Component: component, Configured: true, Phase: "idle", Reason: "尚未检查稳定版本"}}
		u.client = &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 4 {
				return errors.New("下载重定向过多")
			}
			return u.allowedURL(r.URL.String())
		}}
		*slot = u
	}
	return *slot
}
func (u *EngineUpdates) snapshot() EngineUpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}
func (u *EngineUpdates) allowedURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Fragment != "" || parsed.Hostname() == "" {
		return errors.New("更新地址无效")
	}
	if u.allowTestHTTP && strings.HasPrefix(value, u.catalogURL[:strings.LastIndex(u.catalogURL, "/")]+"/") {
		return nil
	}
	if parsed.Scheme != "https" || parsed.Port() != "" {
		return errors.New("更新只允许受信任 HTTPS 地址")
	}
	switch parsed.Hostname() {
	case "youyoudezhuzhu.github.io":
		if strings.HasPrefix(parsed.Path, "/Air2Mi/") {
			return nil
		}
	case "github.com":
		if strings.HasPrefix(parsed.Path, "/youyoudezhuzhu/Air2Mi/releases/download/") {
			return nil
		}
	case "release-assets.githubusercontent.com":
		return nil
	}
	return errors.New("更新地址不在发布源白名单")
}
func decodeCatalog(raw []byte, key ed25519.PublicKey) (EngineCatalog, error) {
	return decodeCatalogKind(raw, key, "engine")
}
func decodeCatalogKind(raw []byte, key ed25519.PublicKey, component string) (EngineCatalog, error) {
	var out EngineCatalog
	var envelope struct {
		Format    string `json:"format"`
		KeyID     string `json:"key_id"`
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	if len(raw) > 256<<10 || json.Unmarshal(raw, &envelope) != nil || envelope.Format != "air2mi-update-v1" || envelope.KeyID != "air2mi-update-v1" {
		return out, errors.New("更新清单格式无效")
	}
	payload, e := base64.RawURLEncoding.Strict().DecodeString(envelope.Payload)
	if e != nil {
		return out, e
	}
	signature, e := base64.RawURLEncoding.Strict().DecodeString(envelope.Signature)
	if e != nil || len(signature) != 64 || !ed25519.Verify(key, payload, signature) {
		return out, errors.New("更新清单签名无效")
	}
	if json.Unmarshal(payload, &out) != nil || out.Schema != 1 || out.App != "air2mi" || out.Component != component || out.Channel != "stable" || out.Sequence < 0 || len(out.Targets) > 8 {
		return out, errors.New("更新清单内容不兼容")
	}
	return out, nil
}
func (u *EngineUpdates) check(ctx context.Context) (EngineUpdateStatus, error) {
	u.mu.Lock()
	if u.working {
		v := u.status
		u.mu.Unlock()
		return v, nil
	}
	u.mu.Unlock()
	fail := func(err error) (EngineUpdateStatus, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		if !u.working {
			u.status = EngineUpdateStatus{Configured: true, Phase: "error", Reason: err.Error()}
		}
		return u.status, err
	}
	if e := u.allowedURL(u.catalogURL); e != nil {
		return fail(e)
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", u.catalogURL, nil)
	if e != nil {
		return fail(e)
	}
	req.Header.Set("Cache-Control", "no-cache")
	response, e := u.client.Do(req)
	if e != nil {
		return fail(fmt.Errorf("检查更新失败: %w", e))
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fail(fmt.Errorf("更新源 HTTP %d", response.StatusCode))
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if e != nil {
		return fail(e)
	}
	catalog, e := decodeCatalogKind(raw, u.owner.public, u.component)
	if e != nil {
		return fail(e)
	}
	u.owner.mu.Lock()
	active := u.owner.active.Manifest
	u.owner.mu.Unlock()
	if u.component == "engine" && catalog.Sequence < active.UpdateSequence {
		return fail(errors.New("拒绝回退到旧更新清单"))
	}
	status := EngineUpdateStatus{Component: u.component, CurrentVersion: active.PackageVersion, Configured: true, Phase: "current", Reason: "已是当前稳定版本"}
	if u.component == "app" {
		status.CurrentVersion = version
	}
	var target EngineTarget
	if !catalog.Published {
		status.Phase = "unpublished"
		status.Reason = catalog.Reason
		if status.Reason == "" {
			status.Reason = "暂未发布可用的稳定内核"
		}
	} else {
		for _, candidate := range catalog.Targets {
			if candidate.Platform == "linux" && candidate.Architecture == "x64" {
				if target.Platform != "" {
					return fail(errors.New("更新清单重复目标"))
				}
				target = candidate
			}
		}
		if target.Platform == "" {
			return fail(errors.New("没有 Linux x64 稳定内核"))
		}
		kind := "engine_zip"
		if u.component == "app" {
			kind = "fpk"
		}
		if target.URL == "" || target.Kind != kind || target.API != "v1" || len(target.Source) != 40 || target.PackageVersion == "" || target.Size < 1 || target.Size > 384<<20 || !validDigest(target.SHA256) || (u.component == "engine" && !validDigest(target.EngineSHA256)) || (u.component == "app" && !validAppVersion(target.AppVersion)) {
			return fail(errors.New("稳定目标元数据不兼容"))
		}
		for _, value := range []string{target.URL, target.FallbackURL} {
			if value != "" {
				if e = u.allowedURL(value); e != nil {
					return fail(e)
				}
			}
		}
		status.AppVersion = target.AppVersion
		status.PackageVersion = target.PackageVersion
		status.KernelVersion = target.KernelVersion
		status.Kind = target.Kind
		status.Total = target.Size
		status.ReleaseID = fmt.Sprintf("%d:%s", catalog.Sequence, target.SHA256)
		if (u.component == "engine" && active.SHA != target.EngineSHA256) || (u.component == "app" && newerAppVersion(target.AppVersion, version)) {
			status.Phase = "available"
			status.Available = true
			status.Reason = "发现可安装的稳定版本"
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.working {
		u.status = status
		u.target = target
		u.sequence = catalog.Sequence
	}
	return u.status, nil
}
func validDigest(value string) bool {
	raw, e := hex.DecodeString(value)
	return e == nil && len(raw) == 32 && value == strings.ToLower(value)
}
func (u *EngineUpdates) begin(release string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.working {
		return errors.New("已有内核更新正在进行")
	}
	if !u.status.Available || release != u.status.ReleaseID {
		return errors.New("版本已变化，请重新检查更新")
	}
	u.working = true
	u.status.Phase = "downloading"
	u.status.Available = false
	u.status.Reason = "正在下载并校验内核"
	target, sequence := u.target, u.sequence
	go func() {
		e := u.install(target, sequence)
		u.mu.Lock()
		defer u.mu.Unlock()
		u.working = false
		if e != nil {
			u.status.Phase = "error"
			u.status.Reason = e.Error()
		} else if u.component == "app" {
			u.status.Phase = "install_ready"
			u.status.Reason = "应用包已校验，请下载后在飞牛应用中心覆盖安装"
		} else {
			u.status.Phase = "installed"
			u.status.Reason = "内核更新完成，配置与身份已保留"
		}
	}()
	return nil
}
func (u *EngineUpdates) install(target EngineTarget, sequence int64) error {
	folder, e := os.MkdirTemp(u.owner.Data, "online-update-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(folder)
	archive := filepath.Join(folder, "update.zip")
	var last error
	for _, source := range []string{target.URL, target.FallbackURL} {
		if source == "" {
			continue
		}
		last = u.download(source, archive, target)
		if last == nil {
			break
		}
	}
	if last != nil {
		return last
	}
	if u.component == "app" {
		content, e := os.ReadFile(archive)
		if e != nil {
			return e
		}
		destination := filepath.Join(u.owner.Data, "verified-app-update.fpk")
		if e = atomicFile(destination, content, 0600); e != nil {
			return e
		}
		u.mu.Lock()
		u.appFile = destination
		u.mu.Unlock()
		return nil
	}
	stage := filepath.Join(folder, "release")
	if e = os.Mkdir(stage, 0700); e != nil {
		return e
	}
	if e = unpackUpdate(archive, stage); e != nil {
		return e
	}
	meta, e := checkManifest(filepath.Join(stage, "engine.run"), filepath.Join(stage, "manifest.json"), filepath.Join(stage, "manifest.sig"), u.owner.public)
	if e != nil {
		return e
	}
	if meta.SHA != target.EngineSHA256 || meta.Source != target.Source || meta.PackageVersion != target.PackageVersion || meta.UpdateSequence != sequence {
		return errors.New("下载版本与签名清单不一致")
	}
	u.mu.Lock()
	u.status.Phase = "installing"
	u.status.Reason = "正在安装并检查内核健康"
	u.mu.Unlock()
	return u.owner.updateFrom(stage)
}
func (u *EngineUpdates) download(source, path string, target EngineTarget) error {
	if e := u.allowedURL(source); e != nil {
		return e
	}
	response, e := u.client.Get(source)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("下载 HTTP %d", response.StatusCode)
	}
	out, e := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	h := sha256.New()
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > target.Size {
				e = errors.New("下载大小超出签名清单")
				break
			}
			if _, e = out.Write(buffer[:n]); e != nil {
				break
			}
			h.Write(buffer[:n])
			u.mu.Lock()
			u.status.Downloaded = total
			u.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			e = readErr
			break
		}
	}
	if e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if total != target.Size || hex.EncodeToString(h.Sum(nil)) != target.SHA256 {
		return errors.New("内核下载 SHA256 或大小不匹配")
	}
	return nil
}
func (s *Server) onlineUpdate(w http.ResponseWriter, r *http.Request) {
	updater := s.m.online()
	if strings.Contains(r.URL.Path, "/app-") {
		updater = s.m.appOnline()
	}
	if r.Method == "GET" {
		status, e := updater.check(r.Context())
		if e != nil {
			jsonOut(w, 502, status)
		} else {
			jsonOut(w, 200, status)
		}
		return
	}
	var request struct {
		ReleaseID string `json:"release_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		jsonOut(w, 400, map[string]any{"error": "更新请求无效"})
		return
	}
	if e := updater.begin(request.ReleaseID); e != nil {
		jsonOut(w, 409, map[string]any{"error": e.Error()})
		return
	}
	jsonOut(w, 202, updater.snapshot())
}

func parseAppVersion(value string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return out, false
			}
		}
		n, e := strconv.Atoi(p)
		if e != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
func validAppVersion(value string) bool { _, ok := parseAppVersion(value); return ok }
func newerAppVersion(candidate, current string) bool {
	a, ok := parseAppVersion(candidate)
	b, valid := parseAppVersion(current)
	if !ok || !valid {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func (s *Server) appUpdateFile(w http.ResponseWriter, r *http.Request) {
	u := s.m.appOnline()
	u.mu.Lock()
	file := u.appFile
	target := u.target
	phase := u.status.Phase
	u.mu.Unlock()
	if phase != "install_ready" || file == "" {
		jsonOut(w, 409, map[string]string{"error": "请先下载并校验应用更新"})
		return
	}
	f, e := os.Open(file)
	if e != nil {
		jsonOut(w, 409, map[string]string{"error": "应用包已清理，请重新检查"})
		return
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	if e != nil || n != target.Size || hex.EncodeToString(h.Sum(nil)) != target.SHA256 {
		jsonOut(w, 409, map[string]string{"error": "应用包完整性校验失败"})
		return
	}
	f.Seek(0, io.SeekStart)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=Air2Mi-"+target.AppVersion+".fpk")
	http.ServeContent(w, r, "Air2Mi.fpk", time.Time{}, f)
}
