/*
Copyright 2025 containeroo.ch

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/json"
	"fmt"
	"os"

	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/yaml"
)

// ProfileSpec represents the typed VPA spec fragment loaded from the profile file.
type ProfileSpec vpaautoscaling.VerticalPodAutoscalerSpec

// Profile wraps a VPA spec with optional metadata.
type Profile struct {
	// NameTemplate optionally overrides the global VPA name template for this profile.
	NameTemplate string `yaml:"nameTemplate,omitempty"`
	// Spec is the inline VerticalPodAutoscaler spec fragment for this profile.
	Spec ProfileSpec `yaml:",inline"`
}

// Config holds all profiles plus the default profile name.
type Config struct {
	// DefaultProfile is the profile name used when workloads request "default".
	DefaultProfile string `yaml:"defaultProfile"`
	// Profiles contains all available profiles keyed by their name.
	Profiles map[string]Profile `yaml:"profiles"`
}

// LoadFile reads a profiles file from disk and returns the parsed config.
func LoadFile(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read profiles file %q: %w", filePath, err)
	}
	return parse(data)
}

// UnmarshalJSON supports inline VPA spec fields and rejects a nested
// "spec" block . It inlines all keys except nameTemplate into the ProfileSpec.
func (p *Profile) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Reject spec field.
	if _, ok := raw["spec"]; ok {
		return fmt.Errorf("profile spec must be provided inline; the spec field is not supported")
	}

	// Parse nameTemplate.
	if v, ok := raw["nameTemplate"]; ok {
		if err := json.Unmarshal(v, &p.NameTemplate); err != nil {
			return err
		}
		delete(raw, "nameTemplate")
	}

	if len(raw) == 0 {
		p.Spec = ProfileSpec{}
		return nil
	}

	// Marshal remaining keys into a JSON object.
	merged, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	// Parse the remaining keys into the spec.
	var spec ProfileSpec
	if err := json.Unmarshal(merged, &spec); err != nil {
		return err
	}

	p.Spec = spec
	return nil
}

// UnmarshalJSON tolerates YAML booleans for updateMode (Off/On) by coercing them
// into string values before decoding into the typed VPA spec. Booleans map to
// the legacy on/off modes: true → "Auto", false → "Off". All other modes (e.g.
// "Recreate", "Initial", "InPlaceOrRecreate") must be provided as strings.
func (p *ProfileSpec) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Coerce boolean updateMode to strings understood by the VPA enum.
	if up, ok := raw["updatePolicy"].(map[string]any); ok {
		if mode, ok := up["updateMode"].(bool); ok {
			// legacy on/off shortcut: true → Auto, false → Off
			if mode {
				up["updateMode"] = string(vpaautoscaling.UpdateModeAuto)
			} else {
				up["updateMode"] = string(vpaautoscaling.UpdateModeOff)
			}
		}
	}

	// Encode spec into JSON.
	normalized, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	// Parse the JSON into the typed spec.
	var spec vpaautoscaling.VerticalPodAutoscalerSpec
	if err := json.Unmarshal(normalized, &spec); err != nil {
		return err
	}

	// Store the normalized spec.
	*p = ProfileSpec(spec)
	return nil
}

// parse unmarshals a profiles YAML document into a Config.
func parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	return &cfg, nil
}
