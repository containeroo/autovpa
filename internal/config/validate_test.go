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
	"testing"

	"github.com/containeroo/autovpa/internal/flag"
	"github.com/containeroo/autovpa/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("Errors when no profiles", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{}
		err := cfg.Validate(flag.DefaultNameTemplate)
		assert.Error(t, err)
	})

	t.Run("Errors when default missing", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			Profiles: map[string]Profile{"p1": {Spec: ProfileSpec{}}},
		}
		err := cfg.Validate(flag.DefaultNameTemplate)
		assert.Error(t, err)
	})

	t.Run("Errors when default not present", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "missing",
			Profiles:       map[string]Profile{"p1": {Spec: ProfileSpec{}}},
		}
		err := cfg.Validate(flag.DefaultNameTemplate)
		assert.Error(t, err)
	})

	t.Run("Passes on valid config", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{}},
			},
		}
		err := cfg.Validate(flag.DefaultNameTemplate)
		require.NoError(t, err)
	})

	t.Run("Catches invalid profile template", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{}, NameTemplate: "UPPER"},
			},
		}
		err := cfg.Validate(flag.DefaultNameTemplate)
		assert.Error(t, err)
	})

	t.Run("Validates default template", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{}},
			},
		}
		err := cfg.Validate("{{ toLower .WorkloadName }}-{{ dnsLabel .Profile }}")
		require.NoError(t, err)
	})

	t.Run("RenderNameTemplate errors on invalid template", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{}},
			},
		}
		err := cfg.Validate("{{ .Invalid }}")
		require.Error(t, err)
		assert.EqualError(t, err, "default name template invalid: render template: template: name:1:3: executing \"name\" at <.Invalid>: can't evaluate field Invalid in type utils.NameTemplateData")
	})

	t.Run("validateProfileSpec errors on targetRef", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{
					TargetRef: &autoscalingv1.CrossVersionObjectReference{
						APIVersion: "v1",
						Kind:       "Deployment",
						Name:       "demo",
					},
				}},
			},
		}
		err := cfg.Validate("{{ .WorkloadName }}")
		require.Error(t, err)
		assert.EqualError(t, err, "profile \"p1\" invalid: invalid profile: .targetRef must not be set")
	})
}

func TestRenderNameTemplateValidation(t *testing.T) {
	t.Parallel()

	t.Run("Rejects empty template", func(t *testing.T) {
		t.Parallel()
		_, err := utils.RenderNameTemplate("", utils.NameTemplateData{
			WorkloadName: "demo",
			Namespace:    "ns",
			Kind:         "Deployment",
			Profile:      "p1",
		})
		require.Error(t, err)
	})

	t.Run("Profile override template validated even if default valid", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{}},
				"p2": {Spec: ProfileSpec{}, NameTemplate: "{{ .Missing }"},
			},
		}
		err := cfg.Validate(flag.DefaultNameTemplate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "profile \"p2\" name template invalid")
	})

	t.Run("Profile override template accepted when valid even if default invalid", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{
			DefaultProfile: "p1",
			Profiles: map[string]Profile{
				"p1": {Spec: ProfileSpec{}, NameTemplate: "{{ toLower .WorkloadName }}-{{ .Profile }}"},
			},
		}
		err := cfg.Validate("{{ .Invalid }}")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "default name template invalid")
	})
}
