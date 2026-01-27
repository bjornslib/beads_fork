package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/rpc"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

var teamCmd = &cobra.Command{
	Use:     "team",
	GroupID: "views",
	Short:   "Show team work distribution by GitHub username",
	Long: `List in-progress issues grouped by their owner's GitHub username.

This command helps visualize how work is distributed across team members
by showing all in-progress issues grouped by the github_username field.

Examples:
  bd team                          # Show all team members with in-progress work
  bd team --filter-team backend    # Filter by team name
  bd team --github-username alice  # Show only alice's work
  bd team --json                   # Output as JSON`,
	Run: runTeam,
}

func init() {
	rootCmd.AddCommand(teamCmd)
	teamCmd.Flags().String("filter-team", "", "Filter by team name")
	teamCmd.Flags().String("github-username", "", "Filter by specific GitHub username")
}

type TeamMemberIssue struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Priority       int    `json:"priority"`
	GitHubUsername string `json:"github_username"`
	Team           string `json:"team,omitempty"`
}

type TeamMember struct {
	GitHubUsername  string            `json:"github_username"`
	InProgressCount int               `json:"in_progress_count"`
	Issues          []TeamMemberIssue `json:"issues"`
}

type TeamOutput struct {
	Members []TeamMember `json:"members"`
	Total   int          `json:"total_issues"`
}

func runTeam(cmd *cobra.Command, args []string) {
	filterTeam, _ := cmd.Flags().GetString("filter-team")
	githubUsername, _ := cmd.Flags().GetString("github-username")

	ctx := rootCtx

	// If daemon is running, use RPC
	if daemonClient != nil {
		runTeamViaDaemon(ctx, daemonClient, filterTeam, githubUsername)
		return
	}

	// Direct mode - check database freshness first
	if err := ensureDatabaseFresh(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	runTeamDirect(ctx, filterTeam, githubUsername)
}

func runTeamViaDaemon(ctx context.Context, client *rpc.Client, filterTeam, githubUsername string) {
	// Build filter for in_progress issues
	listArgs := &rpc.ListArgs{
		Status: "in_progress",
	}

	resp, err := client.List(listArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var issues []*types.Issue
	if err := json.Unmarshal(resp.Data, &issues); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	processAndOutputTeamIssues(issues, filterTeam, githubUsername)
}

func runTeamDirect(ctx context.Context, filterTeam, githubUsername string) {
	// Build filter for in_progress status
	status := types.StatusInProgress
	filter := types.IssueFilter{
		Status: &status,
	}

	issues, err := store.SearchIssues(ctx, "", filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	processAndOutputTeamIssues(issues, filterTeam, githubUsername)
}

func processAndOutputTeamIssues(issues []*types.Issue, filterTeam, filterGHUsername string) {
	// Group by github_username
	memberMap := make(map[string]*TeamMember)

	for _, issue := range issues {
		// Skip issues without github_username
		ghUser := issue.GitHubUsername
		if ghUser == "" {
			continue
		}

		// Apply filters
		if filterTeam != "" && issue.Team != filterTeam {
			continue
		}
		if filterGHUsername != "" && ghUser != filterGHUsername {
			continue
		}

		if member, ok := memberMap[ghUser]; ok {
			member.Issues = append(member.Issues, TeamMemberIssue{
				ID:             issue.ID,
				Title:          issue.Title,
				Status:         string(issue.Status),
				Priority:       issue.Priority,
				GitHubUsername: ghUser,
				Team:           issue.Team,
			})
			member.InProgressCount++
		} else {
			memberMap[ghUser] = &TeamMember{
				GitHubUsername:  ghUser,
				InProgressCount: 1,
				Issues: []TeamMemberIssue{{
					ID:             issue.ID,
					Title:          issue.Title,
					Status:         string(issue.Status),
					Priority:       issue.Priority,
					GitHubUsername: ghUser,
					Team:           issue.Team,
				}},
			}
		}
	}

	// Convert to slice and sort
	var members []TeamMember
	for _, m := range memberMap {
		// Sort issues by priority within each member
		sort.Slice(m.Issues, func(i, j int) bool {
			return m.Issues[i].Priority < m.Issues[j].Priority
		})
		members = append(members, *m)
	}

	// Sort members by issue count (descending)
	sort.Slice(members, func(i, j int) bool {
		return members[i].InProgressCount > members[j].InProgressCount
	})

	totalIssues := 0
	for _, m := range members {
		totalIssues += m.InProgressCount
	}

	output := TeamOutput{
		Members: members,
		Total:   totalIssues,
	}

	// Use global jsonOutput flag
	if jsonOutput {
		outputJSON(output)
		return
	}

	// Pretty print
	displayTeamOutput(output)
}

func displayTeamOutput(output TeamOutput) {
	if len(output.Members) == 0 {
		fmt.Printf("\n%s No team members with in-progress work\n\n", ui.RenderAccent("📊"))
		return
	}

	fmt.Printf("\n%s Team Work Distribution (%d members, %d issues)\n", 
		ui.RenderBold("👥"), len(output.Members), output.Total)
	fmt.Println(strings.Repeat("-", 60))

	for _, m := range output.Members {
		fmt.Printf("\n%s (%d issues):\n", ui.RenderBold(m.GitHubUsername), m.InProgressCount)
		for _, issue := range m.Issues {
			title := truncateTeamStr(issue.Title, 40)
			fmt.Printf("  %s  %-40s  %s\n", 
				ui.RenderID(issue.ID), 
				title, 
				ui.RenderPriority(issue.Priority))
		}
	}
	fmt.Println()
}

func truncateTeamStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Ensure ctx is used to avoid unused variable warning
var _ = context.WithTimeout
var _ = time.Second
