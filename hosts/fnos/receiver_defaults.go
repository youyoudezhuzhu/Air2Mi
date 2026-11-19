package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

type receiverRecord struct {
	ID   string `json:"input_id"`
	Port int    `json:"port"`
}

// Registry IDs and listener ports are Host details, not user-facing settings.
func prepareReceiver(body map[string]any, known []receiverRecord, available func(int) bool) error {
	usedIDs := map[string]bool{}
	usedPorts := map[int]bool{}
	for _, input := range known {
		usedIDs[input.ID] = true
		usedPorts[input.Port] = true
	}
	id, _ := body["input_id"].(string)
	if strings.TrimSpace(id) == "" {
		for {
			id = "ap-" + randomToken()[:16]
			if !usedIDs[id] {
				break
			}
		}
		body["input_id"] = id
	}
	if usedIDs[id] {
		return errors.New("接收器标识已存在")
	}
	port := 0
	if value, ok := body["port"]; ok {
		n, ok := value.(float64)
		if !ok || n != float64(int(n)) || n < 1024 || n > 65535 {
			return errors.New("无效接收器端口")
		}
		port = int(n)
		if usedPorts[port] || !available(port) {
			return errors.New("接收器端口已被占用")
		}
	}
	if port == 0 {
		for candidate := 7100; candidate < 8100; candidate++ {
			if !usedPorts[candidate] && available(candidate) {
				port = candidate
				break
			}
		}
	}
	if port == 0 {
		return errors.New("没有可用的接收器端口，请停止冲突服务后重试")
	}
	body["port"] = port
	if _, ok := body["kind"]; !ok {
		body["kind"] = "airplay"
	}
	return nil
}
func (m *Manager) receiverDefaults(body map[string]any) error {
	raw, err := m.get("/v1/inputs")
	if err != nil {
		return err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var registry struct {
		Inputs []receiverRecord `json:"inputs"`
	}
	if err = json.Unmarshal(data, &registry); err != nil {
		return err
	}
	return prepareReceiver(body, registry.Inputs, func(port int) bool {
		l, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			return false
		}
		l.Close()
		return true
	})
}
