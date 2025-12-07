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

package app

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("Smoke", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		cfg := writeProfileFile(t)
		args := []string{
			"--leader-elect=false",
			"--watch-namespace=test-autovpa",
			"--metrics-enabled=false",
			"--config=" + cfg,
		}
		out := &bytes.Buffer{}

		errCh := make(chan error, 1)
		go func() {
			errCh <- Run(ctx, "v0.0.0", args, out)
		}()

		time.Sleep(2 * time.Second)
		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("Run returned an error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not return within the expected time")
		}
	})

	t.Run("Invalid args", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		args := []string{"--invalid-flag"}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.EqualError(t, err, "error parsing arguments: unknown flag: --invalid-flag")
	})

	t.Run("Request Help", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		args := []string{"--version"}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		assert.NoError(t, err)
		assert.Equal(t, "v0.0.0", out.String())
	})

	t.Run("Logger error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		args := []string{"--log-encoder", "invalid"}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.EqualError(t, err, "error parsing arguments: invalid value for flag --log-encoder: must be one of: json, console.")
	})

	t.Run("Missing profile file", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		args := []string{
			"--config", "/tmp/does-not-exist.yaml",
			"--leader-elect=false",
			"--metrics-enabled=false",
		}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to load profiles: read profiles file")
	})

	t.Run("Wrong profile file", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		path := t.TempDir() + "/profiles.yaml"
		err := os.WriteFile(path, []byte(`
defaultProfile: p1
profiles:
`), 0o644)
		require.NoError(t, err)

		args := []string{
			"--leader-elect=false",
			"--watch-namespace=test-autovpa",
			"--metrics-enabled=false",
			"--config=" + path,
		}

		out := &bytes.Buffer{}

		err = Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.EqualError(t, err, "failed to validate profiles: profiles must be set")
	})

	t.Run("Duplicate keys", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		profilePath := writeProfileFile(t)
		args := []string{
			"--config", profilePath,
			"--profile-annotation", "dup",
			"--managed-label", "dup",
			"--leader-elect=false",
			"--metrics-enabled=false",
		}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.ErrorContains(t, err, "keys must be unique: duplicate key value")
	})

	t.Run("Invalid name template", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		profilePath := writeProfileFile(t)
		args := []string{
			"--config", profilePath,
			"--vpa-name-template", "{{ .Missing",
			"--leader-elect=false",
			"--metrics-enabled=false",
			"--disable-crd-check",
		}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to validate profiles: default name template invalid")
	})

	t.Run("Leader Election", func(t *testing.T) {
		ctx := t.Context()
		cfg := writeProfileFile(t)
		args := []string{
			"--health-probe-bind-address", ":8082",
			"--config=" + cfg,
		}
		out := &bytes.Buffer{}

		err := Run(ctx, "v0.0.0", args, out)

		require.Error(t, err)
		assert.ErrorContains(t, err, "unable to create manager: unable to find leader election namespace")
	})
}

func writeProfileFile(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/profiles.yaml"
	err := os.WriteFile(path, []byte(`
defaultProfile: p1
profiles:
  p1: {}
`), 0o644)
	require.NoError(t, err)
	return path
}
