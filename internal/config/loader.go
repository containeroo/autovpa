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
	NameTemplate string      `yaml:"nameTemplate,omitempty"` // Optional VPA name template override for this profile.
	Spec         ProfileSpec `yaml:",inline"`                // Inline VPA spec fragment.
}

// Config holds all profiles plus the default profile name.
type Config struct {
	DefaultProfile string             `yaml:"defaultProfile"` // Name of the profile to use when workloads request "default".
	Profiles       map[string]Profile `yaml:"profiles"`       // All profiles keyed by name.
}

// Load reads a profiles file from disk and returns the parsed config.
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read profiles file %q: %w", filePath, err)
	}

	cfg := Config{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse profiles file %q: %w", filePath, err)
	}

	return &cfg, nil
}
