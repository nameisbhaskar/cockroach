// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.
package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"github.com/muesli/reflow/truncate"
	"golang.org/x/oauth2"
)

type featureDetails struct {
	productArea       string
	feature           string
	priority          string
	drtCoverageStatus string
}

func main() {
	// GitHub API Token and Repository Details
	githubToken := ""
	owner := "cockroachdb"
	repo := "cockroach"

	// Authentication using GitHub personal access token
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	// List of issues to be created
	issues := make([][]string, 0)

	// Create issues from the list
	for _, issue := range issues {
		fd := &featureDetails{
			productArea:       strings.TrimSpace(issue[0]),
			feature:           strings.TrimSpace(issue[1]),
			priority:          strings.ToLower(strings.TrimSpace(issue[2])),
			drtCoverageStatus: strings.TrimSpace(issue[3]),
		}
		createGitHubIssue(ctx, client, owner, repo, fd)
	}
}

func createGitHubIssue(
	ctx context.Context, client *github.Client, owner, repo string, fd *featureDetails,
) {
	if fd == nil {
		return
	}

	body := fmt.Sprintf("%s\n\nProduct Area: %s\n\n\n(automatically generated issue)", fd.feature, fd.productArea)
	paLabel := strings.ReplaceAll(strings.Split(fd.productArea, " (")[0], " ", "-")
	labels := []string{"O-drt-test-coverage", "C-enhancement", "T-drp",
		parsePriority(fd.priority), parseDRTStatus(fd.drtCoverageStatus),
		fmt.Sprintf("O-drt-tc-a-%s", paLabel),
	}
	title := truncate.String(fd.feature, 39)
	if len(title) != len(fd.feature) {
		title = fmt.Sprintf("%s...", title)
	}
	assignees := make([]string, 0)
	// Create a GitHub Issue Request
	newIssue := &github.IssueRequest{
		Title:     github.String(fmt.Sprintf("drt: %s", title)),
		Body:      github.String(body),
		Labels:    &labels,
		Assignees: &assignees,
	}

	// Create the issue
	createdIssue, _, err := client.Issues.Create(ctx, owner, repo, newIssue)
	if err != nil {
		log.Printf("Failed to create GitHub issue: %v", err)
		return
	}

	fmt.Printf("GitHub issue created: %s (URL: %s)\n", *createdIssue.Title, *createdIssue.HTMLURL)
}

// parseDRTStatus
// 0	No testing
// 1	Some manual testing
// 2	Automated testing
// 3	Integrated into operations framework
// 4	Currently undefined
func parseDRTStatus(status string) string {
	statusInt, err := strconv.ParseInt(status, 10, 64)
	if err != nil {
		statusInt = 4
	}
	return fmt.Sprintf("O-drt-c-%d", statusInt)
}

func parsePriority(priority string) string {
	priorityLabel, ok := map[string]string{
		"high":   "P-1",
		"medium": "P-2",
		"low":    "P-3",
	}[priority]
	if !ok {
		// making P-0 for more visibility
		return "P-0"
	}
	return priorityLabel
}
