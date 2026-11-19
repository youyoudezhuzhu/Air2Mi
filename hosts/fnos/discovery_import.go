package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type discoveredSpeaker struct {
	ID      string         `json:"speaker_id"`
	Name    string         `json:"display_name"`
	IP      string         `json:"ip"`
	Model   any            `json:"model"`
	Enabled bool           `json:"enabled"`
	Options map[string]any `json:"options"`
}

type discoveryRepair struct {
	SpeakerID string
	IP        string
	Model     any
	Options   map[string]any
}

// Repair only by stable device identity. Model, name and IP are not identity:
// same-model speakers and DHCP address reuse make those unsafe migration keys.
func planDiscoveryRepairs(found, known []discoveredSpeaker) []discoveryRepair {
	return planDiscoveryRepairsWithReferences(found, known, nil)
}

func planDiscoveryRepairsWithReferences(found, known []discoveredSpeaker, references map[string]int) []discoveryRepair {
	knownByID := map[string]discoveredSpeaker{}
	knownByDiscoveryID := map[string][]discoveredSpeaker{}
	for _, s := range known {
		knownByID[s.ID] = s
		if key := discoveryKey(s.Options); key != "" {
			knownByDiscoveryID[key] = append(knownByDiscoveryID[key], s)
		}
	}
	usedKnown := map[string]bool{}
	var repairs []discoveryRepair
	for _, f := range found {
		key := discoveryKey(f.Options)
		old, ok := knownByID[f.ID]
		if key != "" {
			matches := knownByDiscoveryID[key]
			var related []discoveredSpeaker
			for _, candidate := range known {
				candidateKey := discoveryKey(candidate.Options)
				if candidateKey == key || nearUUIDDiscoveryID(candidateKey, key) {
					related = append(related, candidate)
				}
			}
			if len(related) > 1 {
				if referenced, foundUnique := uniqueReferencedSpeaker(related, references); foundUnique {
					old, ok = referenced, true
				} else if len(matches) == 1 {
					old, ok = matches[0], true
				} else {
					continue
				}
			} else if len(matches) == 1 {
				old, ok = matches[0], true
			} else if len(related) == 1 && references[related[0].ID] > 0 {
				// A UUID differing by at most two hex nibbles is treated as a
				// likely identity serialization variant only when the existing
				// record is the sole actively-associated candidate.
				old, ok = related[0], true
			} else if !ok || (discoveryKey(old.Options) != "" && discoveryKey(old.Options) != key) {
				continue
			}
		}
		if !ok || usedKnown[old.ID] {
			continue
		}
		usedKnown[old.ID] = true
		if old.IP != f.IP || discoveryKey(old.Options) != discoveryKey(f.Options) || fmt.Sprint(old.Model) != fmt.Sprint(f.Model) {
			repairs = append(repairs, discoveryRepair{
				SpeakerID: old.ID,
				IP:        f.IP,
				Model:     f.Model,
				Options:   mergeDiscoveryOptions(old.Options, f.Options),
			})
		}
	}
	return repairs
}

func nearUUIDDiscoveryID(a, b string) bool {
	if len(a) != 36 || len(b) != 36 || a[8] != '-' || a[13] != '-' || a[18] != '-' || a[23] != '-' || b[8] != '-' || b[13] != '-' || b[18] != '-' || b[23] != '-' {
		return false
	}
	different := 0
	for i := range a {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'F' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'F' {
			y += 'a' - 'A'
		}
		if !isHexNibble(x) || !isHexNibble(y) {
			return false
		}
		if x != y {
			different++
			if different > 2 {
				return false
			}
		}
	}
	return different > 0
}

func isHexNibble(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f'
}

// A duplicate stable identity is repairable only when exactly one registry
// row is referenced by an input or route. This preserves the user's existing
// association and avoids guessing between two live configurations.
func uniqueReferencedSpeaker(matches []discoveredSpeaker, references map[string]int) (discoveredSpeaker, bool) {
	var selected discoveredSpeaker
	count := 0
	for _, speaker := range matches {
		if references[speaker.ID] <= 0 {
			continue
		}
		selected = speaker
		count++
	}
	return selected, count == 1
}

// Only delete unreferenced records when a single referenced row owns the
// stable identity. Ambiguous/unreferenced groups remain untouched.
func planDuplicateSpeakerDeletes(known []discoveredSpeaker, references map[string]int) []string {
	groups := map[string][]discoveredSpeaker{}
	for _, speaker := range known {
		if key := discoveryKey(speaker.Options); key != "" {
			groups[key] = append(groups[key], speaker)
		}
	}
	var remove []string
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		keep, ok := uniqueReferencedSpeaker(group, references)
		if !ok {
			continue
		}
		for _, speaker := range group {
			if speaker.ID != keep.ID && references[speaker.ID] == 0 {
				remove = append(remove, speaker.ID)
			}
		}
	}
	return remove
}

func speakerReferenceCounts(inputs []associationInput, routes []associationRoute) map[string]int {
	counts := map[string]int{}
	for _, input := range inputs {
		if id, ok := input.Options["speaker_id"].(string); ok && id != "" {
			counts[id]++
		}
	}
	for _, route := range routes {
		for _, id := range route.Speakers {
			if id != "" {
				counts[id]++
			}
		}
	}
	return counts
}
func discoveryKey(options map[string]any) string { s, _ := options["discovery_id"].(string); return s }
func mergeDiscoveryOptions(old, found map[string]any) map[string]any {
	out := make(map[string]any, len(old)+len(found))
	for k, v := range old {
		out[k] = v
	}
	for k, v := range found {
		out[k] = v
	}
	return out
}

// Preserve names and settings. Existing records are matched by stable ID and
// address; address migrations are applied before adding any genuinely new item.
func importDiscovered(found, known []discoveredSpeaker, create func(discoveredSpeaker) error) (int, int, error) {
	ids, ips := map[string]bool{}, map[string]bool{}
	for _, speaker := range known {
		ids[speaker.ID] = true
		ips[speaker.IP] = true
	}
	added, skipped := 0, 0
	for _, speaker := range found {
		if speaker.ID == "" || speaker.IP == "" || strings.TrimSpace(speaker.Name) == "" {
			return added, skipped, fmt.Errorf("发现结果缺少设备标识、地址或名称")
		}
		if ids[speaker.ID] || ips[speaker.IP] {
			skipped++
			continue
		}
		if err := create(speaker); err != nil {
			return added, skipped, err
		}
		ids[speaker.ID], ips[speaker.IP] = true, true
		added++
	}
	return added, skipped, nil
}
func (m *Manager) discoverAndAdd() (map[string]any, error) {
	candidates, err := m.discovery()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.get("/v1/speakers")
	if err != nil {
		return nil, fmt.Errorf("请先启动 Engine 后搜索并自动添加音箱: %w", err)
	}
	decode := func(raw any, target any) error {
		b, e := json.Marshal(raw)
		if e != nil {
			return e
		}
		return json.Unmarshal(b, target)
	}
	var found []discoveredSpeaker
	if err = decode(candidates, &found); err != nil {
		return nil, err
	}
	var registry struct {
		Speakers []discoveredSpeaker `json:"speakers"`
	}
	if err = decode(current, &registry); err != nil {
		return nil, err
	}
	refs, err := m.speakerReferenceCounts()
	if err != nil {
		return nil, err
	}
	updated := 0
	for _, repair := range planDiscoveryRepairsWithReferences(found, registry.Speakers, refs) {
		body, _ := json.Marshal(map[string]any{"ip": repair.IP, "model": repair.Model, "options": repair.Options})
		response, e := m.request("/v1/speakers/"+repair.SpeakerID, "PATCH", strings.NewReader(string(body)))
		if e != nil {
			return nil, e
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("更新音箱地址失败 (HTTP %d)", response.StatusCode)
		}
		updated++
	}
	if updated > 0 {
		current, err = m.get("/v1/speakers")
		if err != nil {
			return nil, err
		}
		if err = decode(current, &registry); err != nil {
			return nil, err
		}
	}
	removed := 0
	for _, speakerID := range planDuplicateSpeakerDeletes(registry.Speakers, refs) {
		response, e := m.request("/v1/speakers/"+speakerID, "DELETE", nil)
		if e != nil {
			return nil, e
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("清理重复音箱记录 %s 失败 (HTTP %d)", speakerID, response.StatusCode)
		}
		removed++
	}
	if removed > 0 {
		current, err = m.get("/v1/speakers")
		if err != nil {
			return nil, err
		}
		if err = decode(current, &registry); err != nil {
			return nil, err
		}
	}
	added, skipped, err := importDiscovered(found, registry.Speakers, func(s discoveredSpeaker) error {
		s.Enabled = true
		if s.Options == nil {
			s.Options = map[string]any{}
		}
		b, e := json.Marshal(s)
		if e != nil {
			return e
		}
		response, e := m.request("/v1/speakers", "POST", strings.NewReader(string(b)))
		if e != nil {
			return e
		}
		defer response.Body.Close()
		if response.StatusCode != 201 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			return fmt.Errorf("自动添加 %s 失败 (HTTP %d): %s；已有记录保留，重试将跳过已添加设备", s.Name, response.StatusCode, redact(string(body)))
		}
		return nil
	})
	result := map[string]any{"speakers": candidates, "added": added, "skipped": skipped, "updated": updated, "removed_duplicates": removed, "discovered": len(found), "auto_added": true, "bind": m.Bind, "restart_required": m.runningBind != "" && m.runningBind != m.Bind}
	return result, err
}

func (m *Manager) speakerReferenceCounts() (map[string]int, error) {
	raw, err := m.get("/v1/inputs")
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(raw)
	var inputs struct {
		Inputs []associationInput `json:"inputs"`
	}
	if err = json.Unmarshal(b, &inputs); err != nil {
		return nil, err
	}
	raw, err = m.get("/v1/routes")
	if err != nil {
		return nil, err
	}
	b, _ = json.Marshal(raw)
	var routes struct {
		Routes []associationRoute `json:"routes"`
	}
	if err = json.Unmarshal(b, &routes); err != nil {
		return nil, err
	}
	return speakerReferenceCounts(inputs.Inputs, routes.Routes), nil
}
