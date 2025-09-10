// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package testselector

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/roachtest/spec"
)

const (
	defaultAlertLastRunOn = 5

	// DataAlertTestNameIndex and the following corresponds to the index of the row where the data is returned
	DataAlertTestNameIndex = 0
	DataLastRunIndex       = 1
)

// AllRows are all the rows returned by snowflake. This is used for testing
var AllAlertQueryRows = []string{"name", "last_run"}

//go:embed snowflake_alert_query.sql
var AlertPreparedQuery string

// TestDetails has the details of the test as fetched from snowflake
type AlertTestDetails struct {
	Name            string // test name
	DaysNotRunSince int    // The number of days since the test was last run
}

// AlertTestsReq is the request for CategoriseTests
type AlertTestsReq struct {
	LastRunOn int // number of days to consider for the last time the test is run

	Cloud spec.Cloud // the cloud where the tests were run
	Suite string     // the test suite for which the selection is done
}

// NewDefaultAlertTestsReq returns a new AlertTestsReq with default values populated
func NewDefaultAlertTestsReq(cloud spec.Cloud, suite string) *AlertTestsReq {
	return &AlertTestsReq{
		LastRunOn: defaultAlertLastRunOn,
		Cloud:     cloud,
		Suite:     suite,
	}
}

// IdentifyAlertTests returns the tests identified as not run for LastRunOn days
func IdentifyAlertTests(ctx context.Context, req *AlertTestsReq) ([]*AlertTestDetails, error) {
	db, err := getConnect(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	statement, err := db.Prepare(PreparedQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = statement.Close() }()
	// get the current branch from the teamcity environment
	currentBranch := os.Getenv("TC_BUILD_BRANCH")
	if currentBranch == "" {
		currentBranch = "master"
	}
	// add the parameters in sequence
	rows, err := statement.QueryContext(ctx, currentBranch,
		fmt.Sprintf("%%%s - %s%%", suites[req.Suite], req.Cloud), req.LastRunOn*-1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	// All the column headers
	colHeaders, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	// Will be used to read data while iterating rows.
	colPointers := make([]interface{}, len(colHeaders))
	colContainer := make([]string, len(colHeaders))
	for i := range colPointers {
		colPointers[i] = &colContainer[i]
	}
	// allTestDetails are all the tests that are returned by the snowflake query
	allTestDetails := make([]*AlertTestDetails, 0)
	for rows.Next() {
		err = rows.Scan(colPointers...)
		if err != nil {
			return nil, err
		}
		testInfos := make([]string, len(colContainer))
		copy(testInfos, colContainer)
		// selected columns:
		// 0. test name
		// 1. last time the test was run
		testDetails := &AlertTestDetails{
			Name:            testInfos[DataAlertTestNameIndex],
			DaysNotRunSince: extractDays(testInfos[DataLastRunIndex]),
		}
		allTestDetails = append(allTestDetails, testDetails)
	}
	return allTestDetails, nil
}

func extractDays(timeFromDB string) int {
	var days int
	days, _ = int(time.Since(parseTime(timeFromDB)).Hours()/24), nil
	return days
}

func parseTime(timeStr string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05.000", timeStr)
	return t
}
