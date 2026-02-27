// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package cli

import (
	"fmt"
	"regexp"

	"github.com/cockroachdb/cockroach/pkg/cmd/tef/planners"
)

// matchRegistriesByRegex finds all registries whose plan names match the given regex pattern.
// Returns an error if the pattern is invalid or no plans match.
func matchRegistriesByRegex(
	regexPattern string, registries []planners.Registry,
) ([]planners.Registry, error) {
	// Compile the regex pattern
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Find all registries matching the pattern
	var matchingRegistries []planners.Registry
	for _, r := range registries {
		if regex.MatchString(r.GetPlanName()) {
			matchingRegistries = append(matchingRegistries, r)
		}
	}

	if len(matchingRegistries) == 0 {
		return nil, fmt.Errorf("no plans match the regex pattern: %s", regexPattern)
	}

	return matchingRegistries, nil
}

// matchPlanNamesByRegex finds all plan names that match the given regex pattern.
// Returns an error if the pattern is invalid or no plans match.
func matchPlanNamesByRegex(regexPattern string, registries []planners.Registry) ([]string, error) {
	matchingRegistries, err := matchRegistriesByRegex(regexPattern, registries)
	if err != nil {
		return nil, err
	}

	// Extract plan names from matching registries
	matchingPlans := make([]string, len(matchingRegistries))
	for i, r := range matchingRegistries {
		matchingPlans[i] = r.GetPlanName()
	}

	return matchingPlans, nil
}
