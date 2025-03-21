// This code has been modified from its original form by The Cockroach Authors.
// All modifications are Copyright 2025 The Cockroach Authors.
//
// Copyright 2019 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rafttest

import (
	"strconv"
	"testing"

	"github.com/cockroachdb/datadriven"
)

func (env *InteractionEnv) handleCompact(t *testing.T, d datadriven.TestData) error {
	idx := firstAsNodeIdx(t, d)
	index, err := strconv.ParseUint(d.CmdArgs[1].Key, 10, 64)
	if err != nil {
		return err
	}
	return env.Compact(idx, index)
}

// Compact compacts the given node's log to the supplied log index (inclusive).
func (env *InteractionEnv) Compact(idx int, index uint64) error {
	if err := env.Nodes[idx].Compact(index); err != nil {
		return err
	}
	return env.RaftLog(idx)
}
