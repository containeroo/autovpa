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

		data := []byte(`
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
		data := []byte(`
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
