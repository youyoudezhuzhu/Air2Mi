package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ToneState struct {
	EndReason        string   `json:"end_reason,omitempty"`
	StopRequestedAt  string   `json:"stop_requested_at,omitempty"`
	OriginalVolume   int      `json:"original_device_volume"`
	MaxSeconds       int      `json:"max_seconds,omitempty"`
	ObservedSpeakers []string `json:"observed_speaker_ids,omitempty"`
	ChangedAt        string   `json:"changed_at,omitempty"`
	FirstAudio       bool     `json:"first_audio_confirmed"`
	FramesSent       int      `json:"frames_sent"`
	KernelState      string   `json:"kernel_state,omitempty"`
	ID               string   `json:"id,omitempty"`
	SpeakerID        string   `json:"speaker_id,omitempty"`
	Phase            string   `json:"phase"`
	Error            string   `json:"error,omitempty"`
	DeviceVolume     int      `json:"device_volume,omitempty"`
	StartedAt        string   `json:"started_at,omitempty"`
}
type ToneController struct {
	mu     sync.Mutex
	owner  *Manager
	state  ToneState
	cancel chan struct{}
	done   chan struct{}
}

func (t *ToneController) snapshot() ToneState {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.state
	if s.Phase == "" {
		s.Phase = "idle"
	}
	return s
}
func (t *ToneController) busy() bool { t.mu.Lock(); defer t.mu.Unlock(); return t.done != nil }
func (t *ToneController) phase(phase string) {
	t.mu.Lock()
	t.state.Phase = phase
	t.state.ChangedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.mu.Unlock()
}
func (t *ToneController) stop() error {
	t.mu.Lock()
	done := t.done
	if done == nil {
		t.mu.Unlock()
		return nil
	}
	select {
	case <-t.cancel:
	default:
		close(t.cancel)
	}
	t.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-time.After(40 * time.Second):
		return errors.New("测试音清理尚未结束，拒绝并行启动其他播放")
	}
}
func (m *Manager) toneBusy() bool { return m.tones != nil && m.tones.busy() }
func (m *Manager) toneState() ToneState {
	if m.tones == nil {
		return ToneState{Phase: "idle"}
	}
	return m.tones.snapshot()
}
func guardIdle(state map[string]any) error {
	river, _ := state["river"].(map[string]any)
	instances, _ := state["instances"].(map[string]any)
	if river == nil || instances == nil {
		return errors.New("无法确定当前播放状态，不能启动测试音")
	}
	if producer, ok := river["producer"].(string); ok && producer != "" {
		return errors.New("请先停止所有 AirPlay 投放；测试音会暂时重建接收器入口")
	}
	for _, raw := range instances {
		item, _ := raw.(map[string]any)
		if selected, _ := item["selected"].(bool); selected {
			return errors.New("请先取消所有 AirPlay 音箱选择，再播放测试音")
		}
	}
	if speakers, ok := state["speakers"].([]any); ok {
		for _, raw := range speakers {
			item, _ := raw.(map[string]any)
			if connected, _ := item["connected"].(bool); connected {
				return errors.New("音箱会话尚未退出，请稍候重试")
			}
		}
	}
	return nil
}

// A fixed, deterministic quiet chime asset; no browser-supplied files or paths.
// The long resource accommodates handshaking. The Host stops on user request or after five minutes
// and fails after twenty seconds without first-audio feedback.
const toneSampleRate = 44100
const toneCycleSeconds = 4
const toneAssetSeconds = 330
const toneMaximumDuration = 5 * time.Minute

func tonePCM() []byte {
	data := make([]byte, toneCycleSeconds*toneSampleRate*4)
	frequencies := []float64{440, 660, 880}
	for sample := 0; sample < toneCycleSeconds*toneSampleRate; sample++ {
		within := float64(sample) / toneSampleRate
		slot := int(within / .8)
		local := within - float64(slot)*.8
		value := int16(0)
		if slot < 3 && local < .30 {
			envelope := math.Min(1, math.Min(local/.015, (.30-local)/.015))
			value = int16(32767 * .12 * envelope * math.Sin(2*math.Pi*frequencies[slot]*local))
		}
		offset := sample * 4
		binary.LittleEndian.PutUint16(data[offset:], uint16(value))
		binary.LittleEndian.PutUint16(data[offset+2:], uint16(value))
	}
	return data
}
func toneHeader(bytesCount uint32) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, 36+bytesCount)
	out.WriteString("WAVEfmt ")
	binary.Write(&out, binary.LittleEndian, uint32(16))
	binary.Write(&out, binary.LittleEndian, uint16(1))
	binary.Write(&out, binary.LittleEndian, uint16(2))
	binary.Write(&out, binary.LittleEndian, uint32(toneSampleRate))
	binary.Write(&out, binary.LittleEndian, uint32(toneSampleRate*4))
	binary.Write(&out, binary.LittleEndian, uint16(4))
	binary.Write(&out, binary.LittleEndian, uint16(16))
	out.WriteString("data")
	binary.Write(&out, binary.LittleEndian, bytesCount)
	return out.Bytes()
}
func writeToneAsset(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tone-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	size := toneAssetSeconds * toneSampleRate * 4
	if _, err = f.Write(toneHeader(uint32(size))); err != nil {
		return err
	}
	pattern := tonePCM()
	for remaining := size; remaining > 0; {
		count := len(pattern)
		if count > remaining {
			count = remaining
		}
		n, err := f.Write(pattern[:count])
		if err != nil {
			return err
		}
		if n != count {
			return io.ErrShortWrite
		}
		remaining -= count
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func toneRunContext(parent context.Context, duration time.Duration, stop <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, duration)
	go func() {
		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (m *Manager) payloadRoot() (string, error) {
	entries, err := os.ReadDir(m.cacheDir())
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) != 64 {
			continue
		}
		root := filepath.Join(m.cacheDir(), entry.Name())
		b, err := os.ReadFile(filepath.Join(root, "BUILD.json"))
		if err != nil {
			continue
		}
		var v struct {
			Build  string `json:"build_id"`
			Source string `json:"source_revision"`
		}
		if json.Unmarshal(b, &v) == nil && v.Build == m.active.Manifest.Build && v.Source == m.active.Manifest.Source {
			return root, nil
		}
	}
	return "", errors.New("已安装内核缺少匹配的本地运行时，无法安全读取设备音量")
}
func (m *Manager) physicalVolume(ip string) (int, error) {
	if net.ParseIP(ip) == nil || net.ParseIP(ip).To4() == nil {
		return 0, errors.New("音箱地址无效")
	}
	payload, err := m.payloadRoot()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	// Reuse the installed Engine's read-only query tool. No MiPlay implementation
	// or Python runtime is copied into the App repository.
	cmd := exec.CommandContext(ctx, filepath.Join(payload, "python/bin/python3"), filepath.Join(payload, "app/tools/speaker_volume.py"), "--bind", m.Bind, "--speaker", ip)
	env := os.Environ()
	env = append(env, "PYTHONNOUSERSITE=1", "PYTHONPATH="+filepath.Join(payload, "app")+":"+filepath.Join(payload, "app/kernel"), "LD_LIBRARY_PATH="+filepath.Join(payload, "native/lib")+":"+filepath.Join(payload, "python/lib"))
	cmd.Env = env
	b, err := cmd.Output()
	if err != nil {
		return 0, errors.New("无法读取音箱实际音量，已取消测试，未采用默认音量")
	}
	var value struct {
		Volume int `json:"actual_volume_percent"`
	}
	if json.Unmarshal(b, &value) != nil || value.Volume < 0 || value.Volume > 100 {
		return 0, errors.New("音箱音量回报无效")
	}
	return value.Volume, nil
}
func (m *Manager) cacheDir() string {
	if m.Cache != "" {
		return m.Cache
	}
	return filepath.Join(m.Data, "cache")
}
func (m *Manager) jsonCommand(path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	response, err := m.request(path, "POST", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Engine会话操作失败 HTTP %d: %s", response.StatusCode, redact(string(data)))
	}
	return nil
}
func (t *ToneController) start(speakerID string, choice *int) error {
	m := t.owner
	m.mu.Lock()
	defer m.mu.Unlock()
	t.mu.Lock()
	if t.done != nil {
		t.mu.Unlock()
		return errors.New("已有测试音正在运行")
	}
	t.mu.Unlock()
	if m.active.Binary == "" {
		return errors.New("请先安装并启动 Engine")
	}
	raw, err := m.get("/v1/state")
	if err != nil {
		return err
	}
	state, ok := raw.(map[string]any)
	if !ok {
		return errors.New("Engine状态无效")
	}
	if err = guardIdle(state); err != nil {
		return err
	}
	raw, err = m.get("/v1/speakers")
	if err != nil {
		return err
	}
	b, _ := json.Marshal(raw)
	var registry struct {
		Speakers []discoveredSpeaker `json:"speakers"`
	}
	if err = json.Unmarshal(b, &registry); err != nil {
		return err
	}
	var target *discoveredSpeaker
	for _, speaker := range registry.Speakers {
		if speaker.ID == speakerID {
			copy := speaker
			target = &copy
			break
		}
	}
	if target == nil || !target.Enabled {
		return errors.New("目标音箱不存在或已停用")
	}
	volume, err := m.physicalVolume(target.IP)
	if err != nil {
		return err
	}
	previousVolume := volume
	if choice != nil {
		volume = *choice
	}
	if volume < 1 || volume > 100 {
		return errors.New("请选择1至100%的测试音量")
	}

	// Recheck after the read-only network query to close its race with a phone.
	raw, err = m.get("/v1/state")
	if err != nil {
		return err
	}
	if err = guardIdle(raw.(map[string]any)); err != nil {
		return err
	}
	if m.cachedConfig == nil {
		m.cachedConfig = map[string]any{}
	}
	for _, key := range []string{"inputs", "speakers", "routes"} {
		value, e := m.get("/v1/" + key)
		if e != nil {
			return e
		}
		m.cachedConfig[key] = value
	}
	if err = m.stopLocked(); err != nil {
		return err
	}
	sessionID := "tone-" + randomToken()[:16]
	folder, err := os.MkdirTemp(m.Data, "test-tone-")
	if err != nil {
		restartErr := m.startLocked()
		return fmt.Errorf("创建测试目录失败: %v; 恢复: %v", err, restartErr)
	}
	child := &Manager{Data: folder, Bundle: m.Bundle, Bind: m.Bind, Cache: m.cacheDir(), active: m.active, public: m.public, client: m.client}
	cleanupSetup := func(cause error) error {
		stopErr := child.stopLocked()
		if stopErr != nil {
			t.mu.Lock()
			t.cancel = make(chan struct{})
			close(t.cancel)
			t.done = make(chan struct{})
			t.state = ToneState{ID: sessionID, SpeakerID: speakerID, Phase: "stopping", Error: "初始化失败，正在安全清理", ChangedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			cancel, done := t.cancel, t.done
			t.mu.Unlock()
			go t.monitor(child, folder, sessionID, cancel, done)
			return fmt.Errorf("测试初始化失败 %v，清理失败 %v；正在重试，禁止并行恢复", cause, stopErr)
		}
		os.RemoveAll(folder)
		restartErr := m.startLocked()
		if restartErr != nil {
			return fmt.Errorf("测试失败: %v;恢复原入口失败: %v", cause, restartErr)
		}
		return cause
	}
	asset := filepath.Join(folder, "fixed-test-tone.wav")
	if err = writeToneAsset(asset); err != nil {
		return cleanupSetup(err)
	}
	if err = child.startLocked(); err != nil {
		return cleanupSetup(err)
	}
	// The default daily profile requires AirPlay inputs. File sessions use the
	// existing configurable file profile in this temporary instance only.
	// First bootstrap above creates the verified receiver manifest and Registry.
	if err = child.stopLocked(); err != nil {
		return cleanupSetup(err)
	}
	child.ConfigPath = filepath.Join(folder, "file-session-config.json")
	if err = writeJSON(child.ConfigPath, fileSessionConfig(child)); err != nil {
		return cleanupSetup(err)
	}
	if err = child.startLocked(); err != nil {
		return cleanupSetup(err)
	}
	speaker := map[string]any{"speaker_id": target.ID, "display_name": target.Name, "ip": target.IP, "model": target.Model, "enabled": true, "volume": volume}
	if err = child.jsonCommand("/v1/speakers", speaker); err != nil {
		return cleanupSetup(err)
	}
	if err = child.jsonCommand("/v1/sessions", map[string]any{"session_id": sessionID, "display_name": "Air2Mi 测试音", "kind": "file", "speaker_ids": []string{target.ID}, "file": asset, "loop": false}); err != nil {
		return cleanupSetup(err)
	}
	t.mu.Lock()
	t.cancel = make(chan struct{})
	t.done = make(chan struct{})
	t.state = ToneState{ChangedAt: time.Now().UTC().Format(time.RFC3339Nano), ID: sessionID, SpeakerID: speakerID, Phase: "connecting", DeviceVolume: volume, OriginalVolume: previousVolume, MaxSeconds: 300, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	cancel, done := t.cancel, t.done
	t.mu.Unlock()
	go t.monitor(child, folder, sessionID, cancel, done)
	return nil
}
func (t *ToneController) monitor(child *Manager, folder, id string, cancel, done chan struct{}) {
	start := time.Now()
	ctx, cancelContext := toneRunContext(context.Background(), toneMaximumDuration, cancel)
	defer cancelContext()
	var first time.Time
	var failure error
	endReason := "manual"
	for {
		select {
		case <-cancel:
			goto cleanup
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				endReason = "maximum_duration"
			}
			goto cleanup
		case <-time.After(250 * time.Millisecond):
		}
		raw, err := child.getContext(ctx, "/v1/state")
		if err != nil {
			if ctx.Err() != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					endReason = "maximum_duration"
				}
				break
			}
			failure = err
			break
		}
		state, _ := raw.(map[string]any)
		b, _ := json.Marshal(state["speakers"])
		var outputs []struct {
			ID     string `json:"speaker_id"`
			First  bool   `json:"first_audio_pcm"`
			Phase  string `json:"playback_state"`
			Frames int    `json:"frames_sent"`
		}
		_ = json.Unmarshal(b, &outputs)
		t.mu.Lock()
		if kernel, ok := state["kernel"].(map[string]any); ok {
			t.state.KernelState, _ = kernel["state"].(string)
		}
		target := t.state.SpeakerID
		observed := []string{}
		for _, output := range outputs {
			if output.Frames > 0 {
				observed = append(observed, output.ID)
			}
			if output.ID == target {
				t.state.FirstAudio = t.state.FirstAudio || output.First
				t.state.FramesSent = output.Frames
			}
		}
		t.state.ObservedSpeakers = observed
		t.mu.Unlock()
		wrongTarget := false
		for _, id := range observed {
			if id != target {
				wrongTarget = true
			}
		}
		if wrongTarget {
			failure = errors.New("测试会话出现非目标音箱输出，已停止")
			break
		}
		if first.IsZero() {
			for _, output := range outputs {
				if output.ID == target && output.First && output.Phase == "playing" {
					first = time.Now()
					t.phase("playing")
					break
				}
			}
		}
		if kernel, ok := state["kernel"].(map[string]any); ok && kernel["state"] == "exited" {
			failure = errors.New("Engine文件会话数据面已退出，测试未通过")
			break
		}
		if first.IsZero() && time.Since(start) > 20*time.Second {
			failure = errors.New("20秒内没有收到目标音箱首帧反馈，测试未通过")
			break
		}
	}
cleanup:
	t.mu.Lock()
	t.state.EndReason = endReason
	t.state.StopRequestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if failure != nil {
		t.state.EndReason = "failure"
	}
	t.mu.Unlock()
	t.phase("stopping")
	m := t.owner
	// Retain bounded, local-only diagnostics before session removal replaces logs.
	diagnosticDir := filepath.Join(m.Data, "diagnostics", "test-tone", id)
	_ = filepath.WalkDir(child.Data, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != "bridge.log" && name != "host.log" && name != "status.json" {
			return nil
		}
		f, e := os.Open(path)
		if e != nil {
			return nil
		}
		defer f.Close()
		info, e := f.Stat()
		if e == nil && info.Size() > 128<<10 {
			_, _ = f.Seek(-(128 << 10), io.SeekEnd)
		}
		data, _ := io.ReadAll(io.LimitReader(f, 128<<10))
		safe := []string{}
		for _, line := range strings.Split(string(data), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "key") || strings.Contains(lower, "auth") || strings.Contains(lower, "token") || strings.Contains(lower, "nonce") || strings.Contains(lower, "challenge") {
				continue
			}
			safe = append(safe, line)
		}
		relative, _ := filepath.Rel(child.Data, path)
		_ = atomicFile(filepath.Join(diagnosticDir, relative), []byte(strings.Join(safe, "\n")), 0600)
		return nil
	})
	sessionErr := child.jsonCommand("/v1/sessions/"+id+"/stop", map[string]any{})
	stopErr := child.stopLocked()
	// A failed stop must never release the slot or start a second Engine.
	// Retry cleanup in the background while status/stop remain responsive.
	for stopErr != nil {
		t.mu.Lock()
		t.state.Error = "测试内核清理未完成，正在重试；其他播放操作暂不可用: " + redact(stopErr.Error())
		t.state.ChangedAt = time.Now().UTC().Format(time.RFC3339Nano)
		t.mu.Unlock()
		time.Sleep(2 * time.Second)
		stopErr = child.stopLocked()
	}
	child.client.CloseIdleConnections()
	m.mu.Lock()
	if stopErr == nil {
		os.RemoveAll(folder)
		if err := m.startLocked(); err != nil {
			failure = fmt.Errorf("测试结束但恢复原入口失败: %w", err)
		}
	} else {
		failure = fmt.Errorf("测试内核未安全结束，拒绝并行恢复: %w", stopErr)
	}
	if sessionErr != nil && failure == nil {
		failure = sessionErr
	}
	t.mu.Lock()
	if failure != nil {
		t.state.Phase = "failed"
		t.state.Error = redact(failure.Error())
	} else {
		t.state.Phase = "completed"
		t.state.Error = ""
	}
	t.state.ChangedAt = time.Now().UTC().Format(time.RFC3339Nano)
	receipt := t.state
	t.done = nil
	close(done)
	t.mu.Unlock()
	m.mu.Unlock()
	_ = writeJSON(filepath.Join(m.Data, "diagnostics", "test-tone", id+".json"), receipt)
}
func (s *Server) toneAPI(w http.ResponseWriter, r *http.Request, id string, stop bool) {
	if s.m.tones == nil {
		jsonOut(w, 503, map[string]any{"error": "test_tone_unavailable"})
		return
	}
	var err error
	if stop {
		err = s.m.tones.stop()
	} else {

		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		choice, decodeErr := decodeToneOptions(r.Body)
		if decodeErr != nil {
			jsonOut(w, 400, map[string]any{"error": decodeErr.Error()})
			return
		}
		err = s.m.tones.start(id, choice)
	}
	if err != nil {
		jsonOut(w, 409, map[string]any{"error": err.Error()})
		return
	}
	jsonOut(w, 200, s.m.toneState())
}

func fileSessionConfig(child *Manager) map[string]any {
	hostData := filepath.Join(child.Data, "engine", "host-data")
	return map[string]any{"socket_path": nil, "tcp_host": "127.0.0.1", "tcp_port": 0, "state_path": filepath.Join(hostData, "state.json"), "registry_path": filepath.Join(hostData, "registry.json"), "source": "runtime", "runtime_enabled": true, "bridge_script": filepath.Join(child.Data, "engine", "engine", "tools", "multi_airplay_river_lab.py"), "engine_run_dir": filepath.Join(hostData, "runs"), "receiver_manifest": filepath.Join(hostData, "receiver-build.json"), "identity_dir": filepath.Join(hostData, "identities"), "bridge_bind": child.Bind, "capture_interface": "host-owned", "daily_mode": true, "presentation_delay_ms": nil, "start_at_live_edge": false, "continuous_miplay_session": false, "low_latency_input": false}
}

func decodeToneOptions(body io.Reader) (*int, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.New("测试请求过大")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var options struct {
		Volume *int `json:"volume_percent"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&options); err != nil {
		return nil, errors.New("测试仅接受volume_percent整数音量，不接受文件路径或其他参数")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, errors.New("无效测试请求")
	}
	if options.Volume != nil && (*options.Volume < 1 || *options.Volume > 100) {
		return nil, errors.New("测试音量必须在1至100之间")
	}
	return options.Volume, nil
}
