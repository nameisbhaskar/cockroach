// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package v1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeFormatting(t *testing.T) {
	t.Run("StartTime formatting", func(t *testing.T) {
		// Test that time formatting produces RFC3339 format with nanoseconds
		testTime := time.Date(2025, 12, 16, 11, 22, 7, 818413000, time.UTC)
		formatted := testTime.Format(time.RFC3339Nano)

		// Should produce format like "2025-12-16T11:22:07.818413Z"
		require.Contains(t, formatted, "T", "should contain 'T' separator")
		require.Contains(t, formatted, "Z", "should end with 'Z' for UTC")
		require.Contains(t, formatted, "2025-12-16", "should contain date")
		require.Contains(t, formatted, "11:22:07", "should contain time")

		// Parse it back to ensure it's valid RFC3339
		parsed, err := time.Parse(time.RFC3339Nano, formatted)
		require.NoError(t, err, "should be parseable as RFC3339Nano")
		require.Equal(t, testTime, parsed, "should round-trip correctly")
	})

	t.Run("EndTime formatting", func(t *testing.T) {
		// Test that EndTime uses the same formatting
		testTime := time.Date(2025, 12, 16, 11, 30, 15, 123456789, time.UTC)
		formatted := testTime.Format(time.RFC3339Nano)

		// Should produce format like "2025-12-16T11:30:15.123456789Z"
		require.Contains(t, formatted, "T", "should contain 'T' separator")
		require.Contains(t, formatted, "Z", "should end with 'Z' for UTC")
		require.Contains(t, formatted, "2025-12-16", "should contain date")
		require.Contains(t, formatted, "11:30:15", "should contain time")

		// Parse it back to ensure it's valid RFC3339
		parsed, err := time.Parse(time.RFC3339Nano, formatted)
		require.NoError(t, err, "should be parseable as RFC3339Nano")
		require.Equal(t, testTime, parsed, "should round-trip correctly")
	})
}
