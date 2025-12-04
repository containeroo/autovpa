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

package utils

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestUtilsValidateUniqueKeys(t *testing.T) {
	t.Parallel()

	t.Run("All keys unique", func(t *testing.T) {
		t.Parallel()
		err := ValidateUniqueKeys(map[string]string{
			"a": "x",
			"b": "y",
		})
		assert.NoError(t, err)
	})

	t.Run("Duplicate value triggers error", func(t *testing.T) {
		t.Parallel()
		err := ValidateUniqueKeys(map[string]string{
			"a": "x",
			"b": "x",
		})
		assert.Error(t, err)
	})

	t.Run("Empty map fails", func(t *testing.T) {
		t.Parallel()
		err := ValidateUniqueKeys(map[string]string{})
		assert.Error(t, err)
	})
}

func TestUtilsFormatKeys(t *testing.T) {
	t.Parallel()

	t.Run("Formats sorted keys", func(t *testing.T) {
		t.Parallel()
		out := FormatKeys(map[string]string{
			"b": "2",
			"a": "1",
		})
		assert.Equal(t, "a=1, b=2", out)
	})
}

func TestUtilsDefaultIfZero(t *testing.T) {
	t.Parallel()

	t.Run("Returns default on zero", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 5, DefaultIfZero(0, 5))
	})

	t.Run("Returns value when non-zero", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 3, DefaultIfZero(3, 5))
	})
}

func TestUtilsMergeMaps(t *testing.T) {
	t.Parallel()

	t.Run("Early return on nil base", func(t *testing.T) {
		t.Parallel()
		out := MergeMaps(nil, map[string]string{})
		assert.Empty(t, out)
	})

	t.Run("Merges labels with override", func(t *testing.T) {
		t.Parallel()
		out := MergeMaps(map[string]string{"a": "1", "b": "2"}, map[string]string{"b": "3", "c": "4"})
		assert.Equal(t, map[string]string{"a": "1", "b": "3", "c": "4"}, out)
	})

	t.Run("Handles nil base", func(t *testing.T) {
		t.Parallel()
		out := MergeMaps(nil, map[string]string{"a": "1"})
		assert.Equal(t, map[string]string{"a": "1"}, out)
	})
}

func TestUtilsToCacheOptions(t *testing.T) {
	t.Parallel()

	t.Run("Empty namespaces returns default", func(t *testing.T) {
		t.Parallel()
		opts := ToCacheOptions(nil)
		assert.Equal(t, 0, len(opts.DefaultNamespaces))
	})

	t.Run("Populates namespaces map", func(t *testing.T) {
		t.Parallel()
		opts := ToCacheOptions([]string{"ns1", "ns2"})
		assert.Contains(t, opts.DefaultNamespaces, "ns1")
		assert.Contains(t, opts.DefaultNamespaces, "ns2")
	})

	t.Run("Handles duplicate and spaced namespaces", func(t *testing.T) {
		t.Parallel()
		opts := ToCacheOptions([]string{"ns1", "ns1", " ns2 "})
		assert.Len(t, opts.DefaultNamespaces, 2)
		assert.Contains(t, opts.DefaultNamespaces, "ns1")
		assert.Contains(t, opts.DefaultNamespaces, " ns2 ")
	})
}

func TestUtilsEnsureVPAResource(t *testing.T) {
	t.Parallel()

	t.Run("CRD present", func(t *testing.T) {
		t.Parallel()

		cfg := newDiscoveryConfig(t, true)
		err := EnsureVPAResource(cfg)
		assert.NoError(t, err)
	})

	t.Run("CRD missing", func(t *testing.T) {
		t.Parallel()

		cfg := newDiscoveryConfig(t, false)
		err := EnsureVPAResource(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verticalpodautoscaler CRD not installed")
	})
}

func TestUtilsRenderNameTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Empty template", func(t *testing.T) {
		t.Parallel()
		out, err := RenderNameTemplate("", NameTemplateData{
			WorkloadName: "DemoApp",
			Profile:      "P1",
		})
		require.Error(t, err)
		assert.Empty(t, out)
		assert.EqualError(t, err, "template must not be empty")
	})

	t.Run("Template parse error", func(t *testing.T) {
		t.Parallel()
		out, err := RenderNameTemplate("{{ .Invalid ", NameTemplateData{
			WorkloadName: "DemoApp",
			Profile:      "P1",
		})
		require.Error(t, err)
		assert.Empty(t, out)
		assert.EqualError(t, err, "parse template: template: name:1: unclosed action")
	})

	t.Run("Invalid template", func(t *testing.T) {
		t.Parallel()
		out, err := RenderNameTemplate("{{ .Invalid }}", NameTemplateData{
			WorkloadName: "DemoApp",
			Profile:      "P1",
		})
		require.Error(t, err)
		assert.Empty(t, out)
		assert.EqualError(t, err, "render template: template: name:1:3: executing \"name\" at <.Invalid>: can't evaluate field Invalid in type utils.NameTemplateData")
	})

	t.Run("Renders with helpers", func(t *testing.T) {
		t.Parallel()
		out, err := RenderNameTemplate("{{ toLower .WorkloadName }}-{{ dnsLabel .Profile }}", NameTemplateData{
			WorkloadName: "DemoApp",
			Profile:      "P1",
		})
		require.NoError(t, err)
		assert.Equal(t, "demoapp-p1", out)
	})

	t.Run("Fails on invalid render", func(t *testing.T) {
		t.Parallel()
		_, err := RenderNameTemplate("INVALID", NameTemplateData{WorkloadName: "demo"})
		require.Error(t, err)
	})

	t.Run("Fails DNS validation", func(t *testing.T) {
		t.Parallel()
		_, err := RenderNameTemplate("Demo", NameTemplateData{WorkloadName: "demo"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid DNS-1123 subdomain")
	})

	t.Run("Truncates when using helper", func(t *testing.T) {
		t.Parallel()
		out, err := RenderNameTemplate("{{ truncate .WorkloadName 3 }}-vpa", NameTemplateData{
			WorkloadName: "demoooo",
		})
		require.NoError(t, err)
		assert.Equal(t, "dem-vpa", out)
	})
}

func TestUtilsTruncateRunes(t *testing.T) {
	t.Parallel()

	t.Run("Returns empty when limit non-positive", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", truncateRunes("hello", 0))
		assert.Equal(t, "", truncateRunes("hello", -1))
	})

	t.Run("Returns original when shorter than limit", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "hi", truncateRunes("hi", 5))
	})

	t.Run("Truncates by rune count", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "hé", truncateRunes("héllo", 2))
	})
}

func TestUtilsDNSLabel(t *testing.T) {
	t.Parallel()

	t.Run("Normalizes allowed characters", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "app-1", dnsLabel("App_1"))
	})

	t.Run("Trims disallowed edges", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "mid", dnsLabel("-mid."))
	})

	t.Run("Defaults to vpa on empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "vpa", dnsLabel("???"))
	})
}

type discoveryRoundTripper struct {
	includeVPA bool
}

func (d discoveryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/apis":
		return jsonResponse(apiGroups(d.includeVPA)), nil
	case "/apis/autoscaling.k8s.io/v1":
		if !d.includeVPA {
			return notFoundResponse(), nil
		}
		return jsonResponse(vpaResources()), nil
	case "/api":
		return jsonResponse(&metav1.APIResourceList{GroupVersion: "v1"}), nil
	default:
		return notFoundResponse(), nil
	}
}

func jsonResponse(obj any) *http.Response {
	body, _ := json.Marshal(obj)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func notFoundResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewReader([]byte("not found"))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func apiGroups(includeVPA bool) *metav1.APIGroupList {
	out := &metav1.APIGroupList{}
	if includeVPA {
		out.Groups = append(out.Groups, metav1.APIGroup{
			Name: "autoscaling.k8s.io",
			Versions: []metav1.GroupVersionForDiscovery{{
				GroupVersion: "autoscaling.k8s.io/v1",
				Version:      "v1",
			}},
			PreferredVersion: metav1.GroupVersionForDiscovery{
				GroupVersion: "autoscaling.k8s.io/v1",
				Version:      "v1",
			},
		})
	}
	return out
}

func vpaResources() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "autoscaling.k8s.io/v1",
		APIResources: []metav1.APIResource{
			{
				Name:       "verticalpodautoscalers",
				Namespaced: true,
				Kind:       "VerticalPodAutoscaler",
				Verbs:      metav1.Verbs{"get", "list"},
			},
		},
	}
}

func newDiscoveryConfig(t *testing.T, includeVPA bool) *rest.Config {
	t.Helper()
	return &rest.Config{
		Host:      "http://discovery.invalid",
		Transport: discoveryRoundTripper{includeVPA: includeVPA},
	}
}
