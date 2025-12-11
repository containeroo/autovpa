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
	"os"
	"path/filepath"
	"testing"

	"github.com/containeroo/autovpa/internal/flag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vpaautoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"sigs.k8s.io/yaml"
)

func TestConfigLoadFile(t *testing.T) {
	t.Parallel()

	t.Run("Loads valid profiles file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "profiles.yaml")
		err := os.WriteFile(path, []byte(`
defaultProfile: p1
profiles:
  p1:
    updatePolicy:
      updateMode: "Auto"
`), 0o644)
		require.NoError(t, err)

		cfg, err := LoadFile(path)
		require.NoError(t, err)
		require.NoError(t, cfg.Validate(flag.DefaultNameTemplate))

		assert.Equal(t, "p1", cfg.DefaultProfile)
		_, ok := cfg.Profiles["p1"]
		assert.True(t, ok, "expected profile p1")
	})

	t.Run("Fails when file missing", func(t *testing.T) {
		t.Parallel()
		_, err := LoadFile("/tmp/does-not-exist.yaml")
		assert.Error(t, err)
	})

	t.Run("Fails on invalid YAML", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "profiles.yaml")
		err := os.WriteFile(path, []byte(`:bad yaml`), 0o644)
		require.NoError(t, err)

		_, err = LoadFile(path)
		assert.Error(t, err)
	})
}

func TestConfigParse(t *testing.T) {
	t.Parallel()

	t.Run("Parses valid profiles YAML", func(t *testing.T) {
		t.Parallel()

		data := []byte(`---
defaultProfile: p1
profiles:
  p1:
    updatePolicy:
      updateMode: "Auto"
`)

		cfg, err := parse(data)
		require.NoError(t, err, "expected parse to succeed")
		require.NoError(t, cfg.Validate(flag.DefaultNameTemplate), "expected config to validate")

		assert.Equal(t, "p1", cfg.DefaultProfile)
		_, ok := cfg.Profiles["p1"]
		assert.True(t, ok, "expected profile p1 to be present")
	})

	t.Run("Fails on invalid YAML", func(t *testing.T) {
		t.Parallel()

		data := []byte(`:bad yaml`)

		_, err := parse(data)
		assert.Error(t, err, "expected parse to fail on invalid YAML")
	})

	t.Run("Parses but leaves semantic validation to Validate", func(t *testing.T) {
		t.Parallel()

		// Syntactically valid YAML, but missing required fields for Validate().
		data := []byte(`---
profiles:
  p1:
    updatePolicy:
      updateMode: "Auto"
`)

		cfg, err := parse(data)
		require.NoError(t, err, "parse should succeed for syntactically valid YAML")

		// Validate should now complain because defaultProfile is missing.
		err = cfg.Validate(flag.DefaultNameTemplate)
		assert.Error(t, err, "expected Validate to fail when defaultProfile is missing")
	})
}

func TestProfileUnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("Rejects spec field", func(t *testing.T) {
		t.Parallel()

		data := []byte(`---
defaultProfile: p1
profiles:
  p1:
    spec:
      updatePolicy:
        updateMode: "Auto"
`)

		_, err := parse(data)
		require.Error(t, err, "expected parse to fail when spec is provided")
		assert.Contains(t, err.Error(), "spec field is not supported")
	})

	t.Run("Parses nameTemplate and inline spec fields", func(t *testing.T) {
		t.Parallel()

		data := []byte(`
defaultProfile: p1
profiles:
  p1:
    nameTemplate: "{{ .WorkloadName }}-vpa"
    updatePolicy:
      updateMode: "Initial"
    resourcePolicy:
      containerPolicies:
        - containerName: "*"
          controlledResources: ["cpu", "memory"]
`)

		cfg, err := parse(data)
		require.NoError(t, err)
		require.NoError(t, cfg.Validate(flag.DefaultNameTemplate))

		p := cfg.Profiles["p1"]
		assert.Equal(t, "{{ .WorkloadName }}-vpa", p.NameTemplate)

		mode := p.Spec.UpdatePolicy.UpdateMode
		require.NotNil(t, mode)
		assert.Equal(t, vpaautoscaling.UpdateModeInitial, *mode)

		policies := p.Spec.ResourcePolicy.ContainerPolicies
		require.Len(t, policies, 1)
		assert.Equal(t, "*", policies[0].ContainerName)
		cr := policies[0].ControlledResources
		require.NotNil(t, cr)
		gotResources := make([]string, len(*cr))
		for i, res := range *cr {
			gotResources[i] = string(res)
		}
		assert.ElementsMatch(t, []string{"cpu", "memory"}, gotResources)
	})
}

func TestProfileSpecUnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("Boolean false maps to Off", func(t *testing.T) {
		t.Parallel()

		data := []byte(`
updatePolicy:
  updateMode: Off
`)
		var spec ProfileSpec
		require.NoError(t, yaml.Unmarshal(data, &spec))

		mode := spec.UpdatePolicy.UpdateMode
		require.NotNil(t, mode)
		assert.Equal(t, vpaautoscaling.UpdateModeOff, *mode)
	})

	t.Run("Boolean true maps to Auto", func(t *testing.T) {
		t.Parallel()

		data := []byte(`
updatePolicy:
  updateMode: true
`)
		var spec ProfileSpec
		require.NoError(t, yaml.Unmarshal(data, &spec))

		mode := spec.UpdatePolicy.UpdateMode
		require.NotNil(t, mode)
		assert.Equal(t, vpaautoscaling.UpdateModeAuto, *mode)
	})

	t.Run("Boolean On maps to true", func(t *testing.T) {
		t.Parallel()

		data := []byte(`
updatePolicy:
  updateMode: On
`)
		var spec ProfileSpec
		require.NoError(t, yaml.Unmarshal(data, &spec))

		mode := spec.UpdatePolicy.UpdateMode
		require.NotNil(t, mode)
		assert.Equal(t, vpaautoscaling.UpdateModeAuto, *mode)
	})

	t.Run("Boolean Auto maps to Auto", func(t *testing.T) {
		t.Parallel()

		data := []byte(`
updatePolicy:
  updateMode: Auto
`)
		var spec ProfileSpec
		require.NoError(t, yaml.Unmarshal(data, &spec))

		mode := spec.UpdatePolicy.UpdateMode
		require.NotNil(t, mode)
		assert.Equal(t, vpaautoscaling.UpdateModeAuto, *mode)
	})

	t.Run("Boolean Recreate maps to InPlaceOrRecreate", func(t *testing.T) {
		t.Parallel()

		data := []byte(`
updatePolicy:
  updateMode: InPlaceOrRecreate
`)
		var spec ProfileSpec
		require.NoError(t, yaml.Unmarshal(data, &spec))

		mode := spec.UpdatePolicy.UpdateMode
		require.NotNil(t, mode)
		assert.Equal(t, vpaautoscaling.UpdateModeInPlaceOrRecreate, *mode)
	})
}
