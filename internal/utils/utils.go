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
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"text/template"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// NameTemplateData describes fields available when rendering name templates.
// These map to template variables (.WorkloadName, .Namespace, .Kind, .Profile).
type NameTemplateData struct {
	WorkloadName string
	Namespace    string
	Kind         string
	Profile      string
}

// ValidateUniqueKeys ensures all provided annotation/label values are unique.
// Returns an error if the map is empty or if any duplicate values are found.
func ValidateUniqueKeys(keys map[string]string) error {
	if len(keys) == 0 {
		return errors.New("no keys provided")
	}

	seen := make(map[string]string, len(keys))
	for k, v := range keys {
		if dupKey, ok := seen[v]; ok {
			return fmt.Errorf("duplicate key value %q found for keys %q and %q", v, dupKey, k)
		}
		seen[v] = k
	}
	return nil
}

// FormatKeys returns a sorted, comma-separated string representation of the key/value map.
func FormatKeys(entries map[string]string) string {
	list := make([]string, 0, len(entries))
	for key, val := range entries {
		list = append(list, fmt.Sprintf("%s=%s", key, val))
	}
	sort.Strings(list) // Ensure deterministic ordering
	return strings.Join(list, ", ")
}

// DefaultIfZero returns def when v equals the zero value for T,
// otherwise returns v. Useful for numeric fields where 0 means "unset".
func DefaultIfZero[T comparable](v, def T) T {
	var z T
	if v == z {
		return def
	}
	return v
}

// MergeMaps returns a new map with the a map merged with the b map.
func MergeMaps(a map[string]string, b map[string]string) map[string]string {
	if a == nil && len(b) == 0 {
		return nil
	}

	out := maps.Clone(a)
	if out == nil {
		out = map[string]string{} // Create a new map if a is nil.
	}
	maps.Copy(out, b)
	return out
}

// ToCacheOptions returns cache.Options configured to watch the given namespaces.
// If no namespaces are provided, it returns an empty Options which watches all namespaces.
func ToCacheOptions(watchNamespaces []string) cache.Options {
	if len(watchNamespaces) == 0 {
		return cache.Options{}
	}

	nsMap := make(map[string]cache.Config, len(watchNamespaces))
	for _, ns := range watchNamespaces {
		nsMap[ns] = cache.Config{}
	}

	return cache.Options{DefaultNamespaces: nsMap}
}

// EnsureVPAResource verifies the VerticalPodAutoscaler CRD is installed.
func EnsureVPAResource(restCfg *rest.Config) error {
	disco, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("create discovery client: %w", err)
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))
	_, err = mapper.RESTMapping(schema.GroupKind{Group: "autoscaling.k8s.io", Kind: "VerticalPodAutoscaler"}, "v1")
	if err != nil {
		if meta.IsNoMatchError(err) {
			return fmt.Errorf("verticalpodautoscaler CRD not installed: %w", err)
		}
		return fmt.Errorf("discover verticalpodautoscaler CRD: %w", err)
	}
	return nil
}

// RenderNameTemplate renders and validates the provided template as a DNS-1123 subdomain.
func RenderNameTemplate(tmpl string, data NameTemplateData) (string, error) {
	if strings.TrimSpace(tmpl) == "" {
		return "", errors.New("template must not be empty")
	}

	parsed, err := template.New("name").
		Funcs(template.FuncMap{
			"toLower":  strings.ToLower,
			"replace":  strings.ReplaceAll,
			"trim":     strings.TrimSpace,
			"truncate": truncateRunes,
			"dnsLabel": dnsLabel,
		}).
		Option("missingkey=error").
		Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var rendered strings.Builder
	if err := parsed.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}

	name := rendered.String()
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return "", fmt.Errorf("rendered name %q is not a valid DNS-1123 subdomain: %s", name, strings.Join(errs, ", "))
	}

	return name, nil
}

// truncateRunes trims the string to at most n runes.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i >= n {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// dnsLabel normalizes a string to a DNS-1123-friendly token.
// Valid characters are a-z, 0-9, - and .
func dnsLabel(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "vpa"
	}
	return out
}
