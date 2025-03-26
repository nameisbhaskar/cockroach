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

type Phase struct {
	PhaseName              string `yaml:"phase_name"`
	Description            string `yaml:"description"`
	ExecutionTime          string `yaml:"execution_time,omitempty"`
	ExecutionConfiguration string `yaml:"execution_configuration"`
}

type Phases struct {
	Phases []Phase `yaml:"phases"`
}

func GetPhaseProcessor(ctx context.Context) *cobra.Command {
	cobraCmd := &cobra.Command{
		Use:   "run-pua <yaml file> [flags]",
		Short: "Process the YAML file for the phases",
		Args:  cobra.ExactArgs(1),
		// Wraps the command execution with additional error handling
		Run: helpers.Wrap(func(cmd *cobra.Command, args []string) (retErr error) {
			_, err := exec.LookPath("drtprod")
			if err != nil {
				// drtprod is needed in the path to run yaml commands
				return err
			}
			yamlFileLocation := args[0]
			return processPhaseYamlFile(ctx, yamlFileLocation)
		}),
	}

	return cobraCmd
}

func processPhaseYamlFile(ctx context.Context, yamlFileLocation string) (err error) {
	if _, err = os.Stat(yamlFileLocation); err != nil {
		// if YAML config file is not available, we cannot proceed
		return errors.Wrapf(err, "%s is not present", yamlFileLocation)
	}
	// Read the YAML file from the specified location
	yamlContent, err := os.ReadFile(yamlFileLocation)
	if err != nil {
		return errors.Wrapf(err, "error reading %s", yamlFileLocation)
	}

	return processPhaseYaml(ctx, yamlContent,
		NewFileStateCache(strings.Split(filepath.Base(yamlFileLocation), ".")[0]))
}

func processPhaseYaml(ctx context.Context, yamlContent []byte, c StateCache) (err error) {
	var phases Phases
	if err = yaml.UnmarshalStrict(yamlContent, &phases); err != nil {
		return errors.Wrap(err, "error unmarshalling yaml")
	}
	lastIdx, waitEnd, err := c.Get()
	if err != nil {
		return err
	}
	// Determine the next phase to act upon.
	// In the state file, lastIdx is stored as 1-indexed.
	// If no state file was found, lastIdx remains 0 (i.e. next phase is phases[0]).
	nextPhaseIdx := lastIdx

	// If there's a wait period pending, check if we need to resume.
	if !waitEnd.IsZero() {
		now := time.Now().UTC()
		if now.Before(waitEnd) {
			fmt.Printf("Need to wait for %s at phase '%s'\n", time.Until(waitEnd), phases.Phases[lastIdx-1].PhaseName)
			// Exit without processing anything.
			return nil
		}
		// Waiting period over: print a resume message for the waited phase.
		// Note: When lastIdx > 0, phases[lastIdx-1] was the phase that initiated the wait.
		if nextPhaseIdx > 0 && nextPhaseIdx <= len(phases.Phases) {
			fmt.Printf("Resuming after phase '%s'\n", phases.Phases[nextPhaseIdx-1].PhaseName)
		}
		// Clear waitEnd in state file for the next invocation.
		if err := c.Save(lastIdx, 0); err != nil {
			return fmt.Errorf("failed to update state file: %w", err)
		}
	}

	// No pending wait: process the next phase.
	// Convert 1-indexed stored value to 0-index (nextPhaseIdx==lastIdx).
	if nextPhaseIdx >= len(phases.Phases) {
		// All phases completed: remove the state file.
		if err := c.Remove(); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Println("All phases completed")
		return nil
	}

	phase := phases.Phases[nextPhaseIdx]
	fmt.Printf("Processing phase '%s'\n", phase.PhaseName)
	// Process the YAML file for this phase.
	if err := processYamlFile(ctx, phase.ExecutionConfiguration, false, "", nil); err != nil {
		return fmt.Errorf("failed to process YAML file for phase %s: %w", phase.PhaseName, err)
	}

	// Update state: increment the phase counter.
	newLastIdx := nextPhaseIdx + 1

	// If an execution time is provided, update state with a wait period and exit.
	if phase.ExecutionTime != "" {
		d, err := time.ParseDuration(phase.ExecutionTime)
		if err != nil {
			return fmt.Errorf("invalid execution time for phase %s: %w", phase.PhaseName, err)
		}
		newWaitEnd := time.Now().UTC().Add(d)
		// Update state file with the new phase index and wait end time.
		if err := c.Save(newLastIdx, newWaitEnd.Unix()); err != nil {
			return err
		}
		fmt.Printf("Need to wait for %s at phase %s\n", d, phase.PhaseName)
		return nil
	}
	if newLastIdx >= len(phases.Phases) {
		// All phases completed: remove the state file.
		if err := c.Remove(); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Println("All phases completed")
		return nil
	}
	// No waiting: update the state file without a wait time.
	return c.Save(newLastIdx, 0)
}

type StateCache interface {
	Get() (int, time.Time, error)
	Save(int, int64) error
	Remove() error
}

type FileStateCache struct {
	stateFile string
}

func NewFileStateCache(cacheIdentifier string) StateCache {
	stateFile := fmt.Sprintf(".%s.phase_state.txt", cacheIdentifier)
	return &FileStateCache{stateFile: stateFile}
}

func (s *FileStateCache) Get() (int, time.Time, error) {
	var lastIdx int
	var waitEnd time.Time

	// Read the state file if it exists.
	if content, err := os.ReadFile(s.stateFile); err == nil {
		var waitEndEpoch int64
		// Try parsing "lastIdx,waitStartEpoch".
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

func (s *FileStateCache) Save(newLastIdx int, waitEnd int64) error {
	if err := os.WriteFile(s.stateFile, []byte(fmt.Sprintf("%d,%d", newLastIdx, waitEnd)), 0644); err != nil {
		return fmt.Errorf("failed to update state file: %w", err)
	}
	return nil
}

func (s *FileStateCache) Remove() error {
	if err := os.Remove(s.stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}
	return nil
}
