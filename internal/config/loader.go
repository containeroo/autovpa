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

// parse unmarshals a profiles YAML document into a Config.
func parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	return &cfg, nil
}
