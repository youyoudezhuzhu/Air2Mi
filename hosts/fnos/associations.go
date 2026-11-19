package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

type associationInput struct {
	ID      string         `json:"input_id"`
	Name    string         `json:"display_name"`
	Enabled bool           `json:"enabled"`
	Options map[string]any `json:"options"`
}
type associationRoute struct {
	ID       string   `json:"route_id"`
	Inputs   []string `json:"input_ids"`
	Speakers []string `json:"speaker_ids"`
}

// This validates saved configuration only; it never selects a River producer.
func validateOneToOne(inputs []associationInput, routes []associationRoute) error {
	names := map[string]string{}
	owners := map[string]string{}
	targets := map[string]string{}
	for _, input := range inputs {
		names[input.ID] = input.Name
	}
	claim := func(input, speaker string) error {
		if input == "" || speaker == "" {
			return errors.New("接收器和音箱关联不能为空")
		}
		if name, ok := names[input]; !ok || name == "" {
			return errors.New("路由引用了不存在的接收器")
		}
		if previous, ok := targets[input]; ok && previous != speaker {
			return fmt.Errorf("接收器「%s」只能关联一台音箱", names[input])
		}
		if owner, ok := owners[speaker]; ok && owner != input {
			return fmt.Errorf("该音箱已关联接收器「%s」，请先修改原关联", names[owner])
		}
		targets[input] = speaker
		owners[speaker] = input
		return nil
	}
	for _, input := range inputs {
		if value, ok := input.Options["speaker_id"]; ok {
			speaker, ok := value.(string)
			if !ok || speaker == "" {
				return errors.New("关联音箱标识无效")
			}
			if err := claim(input.ID, speaker); err != nil {
				return err
			}
		}
	}
	for _, route := range routes {
		if len(route.Inputs) != 1 || len(route.Speakers) != 1 {
			return errors.New("每条关联必须只包含一个接收器和一台音箱")
		}
		if err := claim(route.Inputs[0], route.Speakers[0]); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) validateAssociationMutation(kind, method, id string, body map[string]any) error {
	raw, err := m.get("/v1/inputs")
	if err != nil {
		return err
	}
	b, _ := json.Marshal(raw)
	var in struct {
		Inputs []associationInput `json:"inputs"`
	}
	if err = json.Unmarshal(b, &in); err != nil {
		return err
	}
	raw, err = m.get("/v1/routes")
	if err != nil {
		return err
	}
	b, _ = json.Marshal(raw)
	var rt struct {
		Routes []associationRoute `json:"routes"`
	}
	if err = json.Unmarshal(b, &rt); err != nil {
		return err
	}
	if kind == "inputs" {
		if method == "POST" {
			id, _ = body["input_id"].(string)
			name, _ := body["display_name"].(string)
			options, _ := body["options"].(map[string]any)
			if target, _ := options["speaker_id"].(string); target == "" {
				return errors.New("请选择关联音箱")
			}
			in.Inputs = append(in.Inputs, associationInput{ID: id, Name: name, Options: options})
		} else {
			for index := range in.Inputs {
				if in.Inputs[index].ID != id {
					continue
				}
				if value, ok := body["options"]; ok {
					options, valid := value.(map[string]any)
					if !valid {
						return errors.New("关联设置无效")
					}
					in.Inputs[index].Options = options
				}
			}
		}
	} else if kind == "routes" {
		if method == "POST" {
			b, _ = json.Marshal(body)
			var route associationRoute
			if err = json.Unmarshal(b, &route); err != nil {
				return err
			}
			rt.Routes = append(rt.Routes, route)
		} else {
			for index := range rt.Routes {
				if rt.Routes[index].ID != id {
					continue
				}
				b, _ = json.Marshal(body)
				updated := rt.Routes[index]
				if err = json.Unmarshal(b, &updated); err != nil {
					return err
				}
				rt.Routes[index] = updated
			}
		}
	}
	if err = validateOneToOne(in.Inputs, rt.Routes); err != nil {
		return err
	}
	// Validate target existence before changing Engine configuration.
	raw, err = m.get("/v1/speakers")
	if err != nil {
		return err
	}
	b, _ = json.Marshal(raw)
	var sp struct {
		Speakers []struct {
			ID string `json:"speaker_id"`
		} `json:"speakers"`
	}
	if err = json.Unmarshal(b, &sp); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, s := range sp.Speakers {
		known[s.ID] = true
	}
	for _, input := range in.Inputs {
		if target, ok := input.Options["speaker_id"].(string); ok && !known[target] {
			return errors.New("关联音箱不存在，请重新选择")
		}
	}
	return nil
}
