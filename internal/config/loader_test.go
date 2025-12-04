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

func TestConfigLoad(t *testing.T) {
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

		cfg, err := Load(path)
		require.NoError(t, err)
		require.NoError(t, cfg.Validate(flag.DefaultNameTemplate))

		assert.Equal(t, "p1", cfg.DefaultProfile)
		_, ok := cfg.Profiles["p1"]
		assert.True(t, ok, "expected profile p1")
	})

	t.Run("Fails when file missing", func(t *testing.T) {
		t.Parallel()
		_, err := Load("/tmp/does-not-exist.yaml")
		assert.Error(t, err)
	})

	t.Run("Fails on invalid YAML", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "profiles.yaml")
		err := os.WriteFile(path, []byte(`:bad yaml`), 0o644)
		require.NoError(t, err)

		_, err = Load(path)
		assert.Error(t, err)
	})
}
