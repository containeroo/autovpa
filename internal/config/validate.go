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
	"errors"
	"fmt"

	"github.com/containeroo/autovpa/internal/utils"
)

// Validate normalizes profiles, strips targetRef, and ensures defaults exist.
// It also validates that the provided defaultTemplate and per-profile name templates are valid.
func (c *Config) Validate(defaultTemplate string) error {
	if len(c.Profiles) == 0 {
		return errors.New("profiles must be set")
	}
	if c.DefaultProfile == "" {
		return errors.New("defaultProfile must be set")
	}

	// Validate the default name template.
	if _, err := utils.RenderNameTemplate(defaultTemplate, utils.NameTemplateData{
		WorkloadName: "workload",
		Namespace:    "namespace",
		Kind:         "Deployment",
		Profile:      "default",
	}); err != nil {
		return fmt.Errorf("default name template invalid: %w", err)
	}

	// Validate each profile.
	parsed := make(map[string]Profile, len(c.Profiles))
	for name, spec := range c.Profiles {
		copied := copyProfileSpec(spec.Spec)

		// Check if the profile is a valid vericalpodautoscaler.spec.
		if err := validateProfileSpec(&copied); err != nil {
			return fmt.Errorf("profile %q invalid: %w", name, err)
		}

		// Use the default name template if not set.
		templateToValidate := utils.DefaultIfZero(spec.NameTemplate, defaultTemplate)

		// Validate custom name templates.
		if _, err := utils.RenderNameTemplate(templateToValidate, utils.NameTemplateData{
			WorkloadName: "workload",
			Namespace:    "namespace",
			Kind:         "Deployment",
			Profile:      "default",
		}); err != nil {
			return fmt.Errorf("profile %q name template invalid: %w", name, err)
		}

		// Update the parsed map
		parsed[name] = Profile{
			NameTemplate: spec.NameTemplate,
			Spec:         copied,
		}
	}

	// Check if default profile exists.
	if _, ok := parsed[c.DefaultProfile]; !ok {
		return fmt.Errorf("defaultProfile %q not found in profiles", c.DefaultProfile)
	}

	c.Profiles = parsed

	return nil
}
