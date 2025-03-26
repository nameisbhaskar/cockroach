// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/drtprod/helpers"
	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// Phase represents one phase in the YAML configuration.
type Phase struct {
	PhaseName              string `yaml:"phase_name"`               // Name of the phase.
	Description            string `yaml:"description"`              // Description providing details about the phase.
	ExecutionTime          string `yaml:"execution_time,omitempty"` // Optional wait duration after executing this phase.
	ExecutionConfiguration string `yaml:"execution_configuration"`  // YAML file path or configuration for execution.
}

// Phases holds a list of Phase entries.
type Phases struct {
	Phases []Phase `yaml:"phases"` // List of phases to be processed.
}

// GetPhaseProcessor returns a cobra command to process the YAML phases file.
func GetPhaseProcessor(ctx context.Context) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "run-pua <yaml file> [flags]",
		Short: "Process the YAML file for the phases",
		Args:  cobra.ExactArgs(1),
		// Wraps the command execution with additional error handling.
		Run: helpers.Wrap(func(cmd *cobra.Command, args []string) (retErr error) {
			// Ensure that 'drtprod' is available in the system PATH.
			_, err := exec.LookPath("drtprod")
			if err != nil {
				return err
			}
			// Use the provided YAML file location.
			yamlFileLocation := args[0]
			return processPhaseYamlFile(ctx, yamlFileLocation)
		}),
	}

	return cobraCmd
}

// processPhaseYamlFile reads a YAML file from the given path and processes its phases.
func processPhaseYamlFile(ctx context.Context, yamlFileLocation string) (err error) {
	// Check whether the YAML file exists.
	if _, err = os.Stat(yamlFileLocation); err != nil {
		return errors.Wrapf(err, "%s is not present", yamlFileLocation)
	}

	// Read the contents of the YAML file.
	yamlContent, err := os.ReadFile(yamlFileLocation)
	if err != nil {
		return errors.Wrapf(err, "error reading %s", yamlFileLocation)
	}

	// Process the YAML content using a file state cache.
	return processPhaseYaml(ctx, yamlContent,
		NewFileStateCache(strings.Split(filepath.Base(yamlFileLocation), ".")[0]))
}

// processPhaseYaml unmarshals the YAML content and processes the phase logic.
func processPhaseYaml(ctx context.Context, yamlContent []byte, c StateCache) (err error) {
	var phases Phases

	// Unmarshal YAML content into phases struct using strict mode.
	if err = yaml.UnmarshalStrict(yamlContent, &phases); err != nil {
		return errors.Wrap(err, "error unmarshalling yaml")
	}

	// Retrieve the last processed phase index and any pending wait time.
	lastIdx, waitEnd, err := c.Get()
	if err != nil {
		return err
	}

	// nextPhaseIdx is set based on the last completed phase.
	nextPhaseIdx := lastIdx

	// Check if there is a pending wait period before processing next phase.
	if !waitEnd.IsZero() {
		now := time.Now().UTC()
		if now.Before(waitEnd) {
			// Inform the user about the remaining wait time.
			fmt.Printf("Need to wait for %s at phase '%s'\n", time.Until(waitEnd), phases.Phases[lastIdx-1].PhaseName)
			// Exit without processing further.
			return nil
		}
		// Resumption message after waiting period is over.
		if nextPhaseIdx > 0 && nextPhaseIdx <= len(phases.Phases) {
			fmt.Printf("Resuming after phase '%s'\n", phases.Phases[nextPhaseIdx-1].PhaseName)
		}
		// Clear the wait period in the state cache.
		if err := c.Save(lastIdx, 0); err != nil {
			return fmt.Errorf("failed to update state file: %w", err)
		}
	}

	// If all phases have been processed, remove the state file and finish.
	if nextPhaseIdx >= len(phases.Phases) {
		if err := c.Remove(); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Println("All phases completed")
		return nil
	}

	// Process the next phase in sequence.
	phase := phases.Phases[nextPhaseIdx]
	fmt.Printf("Processing phase '%s'\n", phase.PhaseName)

	// Execute the YAML configuration for the current phase.
	if err := processYamlFile(ctx, phase.ExecutionConfiguration, false, "", nil); err != nil {
		return fmt.Errorf("failed to process YAML file for phase %s: %w", phase.PhaseName, err)
	}

	// Increment the phase index to update state.
	newLastIdx := nextPhaseIdx + 1

	// If an execution time is specified, set up a wait period before proceeding.
	if phase.ExecutionTime != "" {
		d, err := time.ParseDuration(phase.ExecutionTime)
		if err != nil {
			return fmt.Errorf("invalid execution time for phase %s: %w", phase.PhaseName, err)
		}
		newWaitEnd := time.Now().UTC().Add(d)
		// Save the updated phase index and wait end time.
		if err := c.Save(newLastIdx, newWaitEnd.Unix()); err != nil {
			return err
		}
		fmt.Printf("Need to wait for %s at phase %s\n", d, phase.PhaseName)
		return nil
	}

	// If this was the last phase, clean up the state file.
	if newLastIdx >= len(phases.Phases) {
		if err := c.Remove(); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Println("All phases completed")
		return nil
	}

	// Update state file with the new phase index when no wait is required.
	return c.Save(newLastIdx, 0)
}

// StateCache interface abstracts state file operations.
type StateCache interface {
	Get() (int, time.Time, error) // Retrieves last completed phase index and pending wait time.
	Save(int, int64) error        // Saves a new phase index and wait end timestamp.
	Remove() error                // Removes the state file.
}

// FileStateCache implements StateCache backed by a file.
type FileStateCache struct {
	stateFile string // File path to store state information.
}

// NewFileStateCache creates a new file state cache with a given identifier.
func NewFileStateCache(cacheIdentifier string) StateCache {
	stateFile := fmt.Sprintf(".%s.phase_state.txt", cacheIdentifier)
	return &FileStateCache{stateFile: stateFile}
}

// Get reads the state file to determine the last processed phase and any wait period.
func (s *FileStateCache) Get() (int, time.Time, error) {
	var lastIdx int
	var waitEnd time.Time

	// Try to read the state file if it exists.
	if content, err := os.ReadFile(s.stateFile); err == nil {
		var waitEndEpoch int64
		// Attempt to parse both phase index and wait period.
		if n, err := fmt.Sscanf(string(content), "%d,%d", &lastIdx, &waitEndEpoch); err == nil && n == 2 {
			if waitEndEpoch != 0 {
				waitEnd = time.Unix(waitEndEpoch, 0).UTC()
			}
		} else if _, err = fmt.Sscanf(string(content), "%d", &lastIdx); err != nil {
			return 0, time.Time{}, fmt.Errorf("could not parse state file: %w", err)
		}
	}
	return lastIdx, waitEnd, nil
}

// Save updates the state file with the new phase index and wait time timestamp.
func (s *FileStateCache) Save(newLastIdx int, waitEnd int64) error {
	// Write new state in the format: "lastIdx,waitEnd".
	if err := os.WriteFile(s.stateFile, []byte(fmt.Sprintf("%d,%d", newLastIdx, waitEnd)), 0644); err != nil {
		return fmt.Errorf("failed to update state file: %w", err)
	}
	return nil
}

// Remove deletes the state file. If file does not exist, it is not treated as an error.
func (s *FileStateCache) Remove() error {
	if err := os.Remove(s.stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}
	return nil
}
