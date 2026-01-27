# Solution Design: Beads Team Collaboration Platform

**Version:** 1.1
**Status:** Final Draft
**Author:** System 3 Meta-Orchestrator
**Date:** 2026-01-27
**Updated:** 2026-01-27 (incorporated user feedback)
**Related PRD:** [team-collaboration-prd.md](../prd/team-collaboration-prd.md)

---

## 1. Executive Summary

This document provides the technical solution design for the Beads Team Collaboration Platform, implementing two primary capabilities:

1. **Continuous Sync** - Async hook-triggered git sync after MCP tool calls
2. **Browser Interface** - Enhanced monitor-webui for team visibility

The design leverages existing beads architecture (daemon, RPC, sync branch) and the new Claude Code async hooks feature to deliver a minimal-change, high-value solution.

---

## 2. Architecture Overview

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Team Collaboration Architecture                      │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐           │
│  │  Claude Code A   │    │  Claude Code B   │    │  Claude Code C   │           │
│  │  (Orchestrator)  │    │  (Worker)        │    │  (Human Session) │           │
│  └────────┬─────────┘    └────────┬─────────┘    └────────┬─────────┘           │
│           │                       │                       │                      │
│           │ MCP                   │ MCP                   │ CLI                  │
│           ▼                       ▼                       ▼                      │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                         Beads MCP Server + CLI                           │    │
│  │  - beads_create_issue()    - bd create                                   │    │
│  │  - beads_update_issue()    - bd update                                   │    │
│  │  - beads_close_issue()     - bd close                                    │    │
│  └──────────────────────────────────┬──────────────────────────────────────┘    │
│                                     │                                           │
│                                     │ RPC                                       │
│                                     ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                           Beads Daemon                                   │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐   │    │
│  │  │ RPC Server  │  │ Auto-Export │  │ Auto-Import │  │ Task Watcher │   │    │
│  │  │             │  │ (500ms deb) │  │ (fsnotify)  │  │ (optional)   │   │    │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────────────┘   │    │
│  │         │                │                │                             │    │
│  │         └────────────────┴────────┬───────┘                             │    │
│  │                                   │                                      │    │
│  │                            ┌──────┴──────┐                               │    │
│  │                            │   SQLite    │                               │    │
│  │                            │  .beads/    │                               │    │
│  │                            │  beads.db   │                               │    │
│  │                            └──────┬──────┘                               │    │
│  └───────────────────────────────────┼─────────────────────────────────────┘    │
│                                      │                                          │
│                                      │ Auto-export                              │
│                                      ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                          JSONL + Git Layer                               │    │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │    │
│  │  │  .beads/issues.jsonl                                            │   │    │
│  │  │  - Git tracked                                                  │   │    │
│  │  │  - 3-way merge on conflict                                      │   │    │
│  │  └──────────────────────────────────┬──────────────────────────────┘   │    │
│  │                                     │                                   │    │
│  │  ┌──────────────────────────────────┴──────────────────────────────┐   │    │
│  │  │  Async Sync Hook (NEW)                                          │   │    │
│  │  │  - Triggered by PostToolUse (async: true)                       │   │    │
│  │  │  - Runs: bd sync (export → commit → push)                       │   │    │
│  │  │  - Non-blocking to Claude Code                                  │   │    │
│  │  └──────────────────────────────────┬──────────────────────────────┘   │    │
│  │                                     │                                   │    │
│  │                                     │ git push                          │    │
│  │                                     ▼                                   │    │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │    │
│  │  │  Remote Repository (GitHub/GitLab)                              │   │    │
│  │  │  - beads_sync branch                                            │   │    │
│  │  │  - Team-visible history                                         │   │    │
│  │  └─────────────────────────────────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
│                                      │                                          │
│                                      │ Webhook / Poll                           │
│                                      ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                        Enhanced Monitor WebUI (NEW)                      │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐   │    │
│  │  │ Active Work │  │ Issue List  │  │ Dep Graph   │  │ Team Stats   │   │    │
│  │  │ Panel       │  │ Table       │  │ D3.js       │  │ Dashboard    │   │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └──────────────┘   │    │
│  │                                                                         │    │
│  │  WebSocket: Real-time updates from daemon                              │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Component Responsibilities

| Component | Responsibility | Changes Required |
|-----------|----------------|------------------|
| **Beads MCP Server** | Handle AI agent requests | Add optimistic locking |
| **Beads Daemon** | Auto-sync, RPC, mutations | Add active session tracking |
| **Async Sync Hook** | Trigger git push on tool use | **NEW** - implement |
| **JSONL/Git Layer** | Distributed sync via git | Add new fields to export |
| **Monitor WebUI** | Team dashboard | **EXTEND** - add panels |

---

## 3. Goal #1: Continuous Sync Implementation

### 3.1 Async Hook Design

#### 3.1.1 Hook Script

**File:** `.claude/hooks/beads-team-sync.sh`

```bash
#!/bin/bash
#===============================================================================
# beads-team-sync.sh - Async PostToolUse hook for team collaboration
#
# Triggered after any beads MCP tool call (create, update, close, etc.)
# Runs in background (async: true) - does NOT block Claude Code execution
#
# Features:
# - 10-second debounce to avoid flooding GitHub with rapid commits
# - Concurrent sync protection via lockfile
# - Idempotent operation (checks for changes before syncing)
# - Logging for debugging
# - Optional webhook notification to dashboard
#
# IMPORTANT: This hook ONLY runs when beads tools are called.
# The matcher pattern "mcp__beads*|mcp__plugin_beads*" ensures
# non-beads MCP tool calls do NOT trigger this hook.
#===============================================================================

set -o pipefail

# Configuration (can be overridden via environment)
BEADS_DIR="${BEADS_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)/.beads}"
SYNC_LOG="${BEADS_SYNC_LOG:-${HOME}/.beads/team-sync.log}"
LOCK_FILE="${BEADS_DIR}/.team-sync.lock"
DEBOUNCE_FILE="${BEADS_DIR}/.sync-debounce"
DEBOUNCE_SECONDS="${BEADS_SYNC_DEBOUNCE:-10}"
LOCK_TIMEOUT="${BEADS_SYNC_LOCK_TIMEOUT:-30}"
MAX_LOG_SIZE="${BEADS_SYNC_MAX_LOG_SIZE:-10485760}"  # 10MB

# Ensure log directory exists
mkdir -p "$(dirname "$SYNC_LOG")"

# Log rotation
if [ -f "$SYNC_LOG" ] && [ "$(stat -f%z "$SYNC_LOG" 2>/dev/null || stat -c%s "$SYNC_LOG" 2>/dev/null)" -gt "$MAX_LOG_SIZE" ]; then
    mv "$SYNC_LOG" "${SYNC_LOG}.old"
fi

log() {
    echo "$(date -u +%FT%TZ) [$$] $*" >> "$SYNC_LOG"
}

log "Hook triggered"

# Check if beads directory exists
if [ ! -d "$BEADS_DIR" ]; then
    log "No beads directory found at $BEADS_DIR, skipping sync"
    exit 0
fi

# Record this change timestamp for debounce
echo "$(date +%s)" > "$DEBOUNCE_FILE"
log "Change detected, debounce timer reset"

# Concurrent sync protection - check if another sync is already running
if [ -f "$LOCK_FILE/pid" ]; then
    holder_pid=$(cat "$LOCK_FILE/pid" 2>/dev/null)
    if [ -n "$holder_pid" ] && kill -0 "$holder_pid" 2>/dev/null; then
        log "Sync already in progress (PID $holder_pid), exiting (debounce file updated)"
        exit 0
    fi
fi

# Wait for debounce period (10 seconds of quiet)
log "Waiting for ${DEBOUNCE_SECONDS}s debounce period..."
while true; do
    sleep 2

    if [ ! -f "$DEBOUNCE_FILE" ]; then
        log "Debounce file removed, exiting"
        exit 0
    fi

    last_change=$(cat "$DEBOUNCE_FILE" 2>/dev/null || echo "0")
    now=$(date +%s)
    elapsed=$((now - last_change))

    if [ $elapsed -ge $DEBOUNCE_SECONDS ]; then
        log "Debounce period elapsed (${elapsed}s >= ${DEBOUNCE_SECONDS}s), proceeding"
        break
    fi

    log "Still in debounce period (${elapsed}s < ${DEBOUNCE_SECONDS}s), waiting..."
done

# Acquire lock for sync
acquire_lock() {
    local start_time=$(date +%s)
    while true; do
        if mkdir "$LOCK_FILE" 2>/dev/null; then
            echo $$ > "$LOCK_FILE/pid"
            trap 'rm -rf "$LOCK_FILE"' EXIT
            log "Lock acquired"
            return 0
        fi

        # Check if lock holder is still alive
        if [ -f "$LOCK_FILE/pid" ]; then
            local holder_pid=$(cat "$LOCK_FILE/pid" 2>/dev/null)
            if [ -n "$holder_pid" ] && ! kill -0 "$holder_pid" 2>/dev/null; then
                log "Stale lock detected (PID $holder_pid dead), removing"
                rm -rf "$LOCK_FILE"
                continue
            fi
        fi

        # Check timeout
        local elapsed=$(($(date +%s) - start_time))
        if [ $elapsed -ge $LOCK_TIMEOUT ]; then
            log "Lock timeout after ${elapsed}s, giving up"
            exit 0
        fi

        sleep 0.5
    done
}

acquire_lock

# Check if sync is needed
sync_status=$(bd sync --status 2>&1)
if ! echo "$sync_status" | grep -q "changes pending\|uncommitted\|unpushed"; then
    log "No changes pending, skipping sync"
    exit 0
fi

log "Changes detected, starting sync"

# Perform sync
sync_output=$(bd sync 2>&1)
sync_exit=$?

if [ $sync_exit -eq 0 ]; then
    log "Sync completed successfully"

    # Optional: Notify dashboard via webhook
    if [ -n "$BEADS_DASHBOARD_WEBHOOK" ]; then
        log "Notifying dashboard webhook"
        curl -s -X POST "$BEADS_DASHBOARD_WEBHOOK" \
            -H "Content-Type: application/json" \
            -d "{\"event\":\"sync_complete\",\"timestamp\":\"$(date -u +%FT%TZ)\",\"source\":\"async-hook\"}" \
            >> "$SYNC_LOG" 2>&1 &
    fi
else
    log "Sync failed (exit $sync_exit): $sync_output"
fi

exit 0
```

#### 3.1.2 Hook Registration

**File:** `.claude/settings.json` (user-level) or `.claude/hooks/config.json` (project-level)

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__beads*|mcp__plugin_beads*|mcp__beads_dev*",
        "hooks": [
          {
            "type": "command",
            "command": ".claude/hooks/beads-team-sync.sh",
            "async": true,
            "timeout": 60
          }
        ]
      }
    ]
  }
}
```

**Installation command:**

```bash
# Add to bd setup claude
bd setup claude --team-sync

# Installs:
# 1. beads-team-sync.sh hook script
# 2. Hook configuration in settings.json
# 3. Creates ~/.beads/ directory for logs
```

### 3.2 Optimistic Locking

#### 3.2.1 MCP Server Enhancement

**File:** `integrations/beads-mcp/src/beads_mcp/tools.py`

```python
from datetime import datetime
from typing import Optional
import json

class ConflictError(Exception):
    """Raised when optimistic lock fails due to concurrent modification."""
    def __init__(self, issue_id: str, expected: str, actual: str):
        self.issue_id = issue_id
        self.expected = expected
        self.actual = actual
        super().__init__(f"Conflict: {issue_id} modified since {expected}")


async def beads_update_issue(
    issue_id: str,
    status: Optional[str] = None,
    priority: Optional[int] = None,
    title: Optional[str] = None,
    description: Optional[str] = None,
    assignee: Optional[str] = None,
    labels: Optional[list[str]] = None,
    expected_updated_at: Optional[str] = None,  # Optimistic lock
    workspace_root: Optional[str] = None,
) -> dict:
    """
    Update a beads issue with optional optimistic locking.

    Args:
        issue_id: The issue ID (e.g., "bd-f7k2")
        status: New status (open, in_progress, closed, etc.)
        priority: New priority (0-4)
        title: New title
        description: New description
        assignee: New assignee
        labels: New labels (replaces existing)
        expected_updated_at: ISO timestamp for optimistic locking
        workspace_root: Override workspace path

    Returns:
        Updated issue dict or error dict

    Raises:
        ConflictError: When optimistic lock fails
    """
    client = await _get_client(workspace_root)

    # Fetch current state for conflict check
    if expected_updated_at:
        try:
            current = await client.show(issue_id)
            current_updated = current.get("updated_at", "")

            # Normalize timestamps for comparison
            if current_updated and expected_updated_at:
                # Parse and compare (handle timezone differences)
                current_dt = datetime.fromisoformat(current_updated.replace("Z", "+00:00"))
                expected_dt = datetime.fromisoformat(expected_updated_at.replace("Z", "+00:00"))

                if current_dt > expected_dt:
                    return {
                        "error": "CONFLICT",
                        "code": "OPTIMISTIC_LOCK_FAILED",
                        "message": f"Issue {issue_id} was modified by another session",
                        "issue_id": issue_id,
                        "expected_updated_at": expected_updated_at,
                        "current_updated_at": current_updated,
                        "current_status": current.get("status"),
                        "hint": "Refresh issue state with beads_show_issue() before retrying"
                    }
        except Exception as e:
            # If we can't verify, proceed with update (fail-open)
            pass

    # Build update arguments
    update_args = {"issue_id": issue_id}
    if status is not None:
        update_args["status"] = status
    if priority is not None:
        update_args["priority"] = priority
    if title is not None:
        update_args["title"] = title
    if description is not None:
        update_args["description"] = description
    if assignee is not None:
        update_args["assignee"] = assignee
    if labels is not None:
        update_args["labels"] = labels

    # Track active session when status changes to in_progress
    if status == "in_progress":
        session_id = os.environ.get("CLAUDE_SESSION_ID", "")
        if session_id:
            update_args["active_session"] = session_id
            update_args["active_session_started_at"] = datetime.utcnow().isoformat() + "Z"

    # Clear active session when status changes from in_progress
    elif status and status != "in_progress":
        update_args["active_session"] = None
        update_args["active_session_started_at"] = None

    return await client.update(**update_args)
```

#### 3.2.2 CLI Support

**File:** `cmd/bd/update.go` (enhancement)

```go
// Add flags for optimistic locking
updateCmd.Flags().String("expect-updated-at", "", "Optimistic lock: reject if issue was modified after this timestamp")
updateCmd.Flags().Bool("force", false, "Force update even if conflict detected")

func runUpdate(cmd *cobra.Command, args []string) error {
    expectUpdatedAt, _ := cmd.Flags().GetString("expect-updated-at")
    force, _ := cmd.Flags().GetBool("force")

    if expectUpdatedAt != "" && !force {
        // Fetch current state
        issue, err := store.GetIssue(issueID)
        if err != nil {
            return err
        }

        // Compare timestamps
        expectedTime, err := time.Parse(time.RFC3339, expectUpdatedAt)
        if err != nil {
            return fmt.Errorf("invalid timestamp format: %s", expectUpdatedAt)
        }

        if issue.UpdatedAt.After(expectedTime) {
            return fmt.Errorf("CONFLICT: issue %s was modified at %s (expected %s). Use --force to override",
                issueID, issue.UpdatedAt.Format(time.RFC3339), expectUpdatedAt)
        }
    }

    // Proceed with update...
}
```

### 3.3 Active Session Tracking

#### 3.3.1 Database Schema

**File:** `internal/storage/sqlite/migrations/036_team_collaboration.go`

```go
package migrations

import "database/sql"

func init() {
    migrations = append(migrations, Migration{
        Version: 36,
        Name:    "team_collaboration",
        Up: func(db *sql.DB) error {
            _, err := db.Exec(`
                -- Ownership tracking (GitHub username for team visibility)
                ALTER TABLE issues ADD COLUMN github_username TEXT;

                -- Last sync metadata
                ALTER TABLE issues ADD COLUMN last_synced_at TIMESTAMP;
                ALTER TABLE issues ADD COLUMN sync_source TEXT;

                -- Team grouping (optional)
                ALTER TABLE issues ADD COLUMN team TEXT;

                -- Performance indexes
                CREATE INDEX IF NOT EXISTS idx_issues_github_username
                    ON issues(github_username)
                    WHERE github_username IS NOT NULL;

                CREATE INDEX IF NOT EXISTS idx_issues_team
                    ON issues(team)
                    WHERE team IS NOT NULL;

                CREATE INDEX IF NOT EXISTS idx_issues_status_priority
                    ON issues(status, priority)
                    WHERE status IN ('open', 'in_progress');
            `)
            return err
        },
        Down: func(db *sql.DB) error {
            // SQLite doesn't support DROP COLUMN, so we'd need to recreate the table
            // For simplicity, Down migration is a no-op
            return nil
        },
    })
}
```

**Note:** The original `active_session`, `active_session_started_at`, and `active_session_agent` fields have been **deferred** because we cannot reliably identify which Claude session is making changes without `CLAUDE_SESSION_ID` being set. The Actor field (tracked via `BD_ACTOR` env var) provides sufficient "who last touched this" tracking.

#### 3.3.2 Types Extension

**File:** `internal/types/types.go` (additions)

```go
type Issue struct {
    // ... existing fields ...

    // Team collaboration fields
    GitHubUsername string     `json:"github_username,omitempty"` // Owner's GitHub username
    LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
    SyncSource     string     `json:"sync_source,omitempty"`     // 'local', 'remote:<machine>'
    Team           string     `json:"team,omitempty"`            // Optional team grouping
}
```

#### 3.3.3 Team Command (Ownership View)

**File:** `cmd/bd/team.go` (new)

```go
package main

import (
    "fmt"

    "github.com/spf13/cobra"
)

var teamCmd = &cobra.Command{
    Use:   "team",
    Short: "List issues by team member (GitHub username)",
    Long:  `Show all in-progress issues grouped by their owner's GitHub username.`,
    RunE:  runTeam,
}

func init() {
    rootCmd.AddCommand(teamCmd)
    teamCmd.Flags().String("filter-team", "", "Filter by team name")
    teamCmd.Flags().String("github-username", "", "Filter by specific GitHub username")
    teamCmd.Flags().Bool("json", false, "Output as JSON")
}

func runTeam(cmd *cobra.Command, args []string) error {
    store, err := getStore()
    if err != nil {
        return err
    }

    filterTeam, _ := cmd.Flags().GetString("filter-team")
    githubUsername, _ := cmd.Flags().GetString("github-username")
    jsonOutput, _ := cmd.Flags().GetBool("json")

    // Query issues by GitHub username
    query := `
        SELECT id, title, status, priority, github_username, team
        FROM issues
        WHERE status = 'in_progress'
        AND github_username IS NOT NULL
        AND github_username != ''
    `
    queryArgs := []interface{}{}

    if filterTeam != "" {
        query += " AND team = ?"
        queryArgs = append(queryArgs, filterTeam)
    }

    if githubUsername != "" {
        query += " AND github_username = ?"
        queryArgs = append(queryArgs, githubUsername)
    }

    query += " ORDER BY github_username, priority"

    rows, err := store.UnderlyingDB().Query(query, queryArgs...)
    if err != nil {
        return err
    }
    defer rows.Close()

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

    memberMap := make(map[string]*TeamMember)
    for rows.Next() {
        var issue TeamMemberIssue
        if err := rows.Scan(&issue.ID, &issue.Title, &issue.Status, &issue.Priority, &issue.GitHubUsername, &issue.Team); err != nil {
            return err
        }

        if member, ok := memberMap[issue.GitHubUsername]; ok {
            member.Issues = append(member.Issues, issue)
            member.InProgressCount++
        } else {
            memberMap[issue.GitHubUsername] = &TeamMember{
                GitHubUsername:  issue.GitHubUsername,
                InProgressCount: 1,
                Issues:          []TeamMemberIssue{issue},
            }
        }
    }

    // Convert to slice
    var members []TeamMember
    for _, m := range memberMap {
        members = append(members, *m)
    }

    if jsonOutput {
        return outputJSON(map[string]interface{}{"members": members})
    }

    // Pretty print
    if len(members) == 0 {
        fmt.Println("No team members with in-progress work")
        return nil
    }

    fmt.Printf("Team Work Summary (%d members)\n\n", len(members))
    for _, m := range members {
        fmt.Printf("%s (%d issues):\n", m.GitHubUsername, m.InProgressCount)
        for _, issue := range m.Issues {
            fmt.Printf("  %-10s  %-30s  [P%d]\n", issue.ID, truncate(issue.Title, 30), issue.Priority)
        }
        fmt.Println()
    }

    return nil
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen-3] + "..."
}
```
```

---

## 4. Goal #2: Browser Interface Implementation

### 4.1 Enhanced Monitor WebUI

#### 4.1.1 New API Endpoints

**File:** `examples/monitor-webui/main.go` (additions)

```go
func main() {
    // ... existing setup ...

    // New team collaboration endpoints
    http.HandleFunc("/api/active", handleAPIActive)
    http.HandleFunc("/api/deps/graph", handleAPIDepsGraph)
    http.HandleFunc("/api/sync/status", handleAPISyncStatus)
    http.HandleFunc("/api/teams", handleAPITeams)
    http.HandleFunc("/webhook", handleWebhook)

    // ... existing server start ...
}

// handleAPITeam returns team members and their in-progress work (by GitHub username)
func handleAPITeam(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Query via RPC - issues grouped by github_username
    resp, err := daemonClient.Call(rpc.OpList, map[string]interface{}{
        "filter": map[string]interface{}{
            "status":             "in_progress",
            "has_github_username": true,
        },
        "group_by": "github_username",
    })
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

// handleAPIDepsGraph returns the full dependency graph
func handleAPIDepsGraph(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    rootID := r.URL.Query().Get("root")

    resp, err := daemonClient.Call(rpc.OpDepTree, map[string]interface{}{
        "issue_id": rootID,
        "format":   "graph", // Returns nodes + edges for D3.js
    })
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

// handleAPISyncStatus returns sync state
func handleAPISyncStatus(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Check git status
    status := struct {
        PendingChanges  bool   `json:"pending_changes"`
        LastSyncAt      string `json:"last_sync_at"`
        SyncBranch      string `json:"sync_branch"`
        RemoteStatus    string `json:"remote_status"`
    }{}

    // Get sync status from daemon
    resp, err := daemonClient.Call(rpc.OpSyncStatus, nil)
    if err == nil {
        if data, ok := resp.(map[string]interface{}); ok {
            status.PendingChanges, _ = data["pending_changes"].(bool)
            status.LastSyncAt, _ = data["last_sync_at"].(string)
            status.SyncBranch, _ = data["sync_branch"].(string)
            status.RemoteStatus, _ = data["remote_status"].(string)
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
}

// handleWebhook receives sync notifications
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var payload struct {
        Event     string `json:"event"`
        Timestamp string `json:"timestamp"`
        Source    string `json:"source"`
    }

    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Broadcast to WebSocket clients
    msg, _ := json.Marshal(map[string]interface{}{
        "type": "sync_notification",
        "data": payload,
    })
    wsBroadcast <- msg

    w.WriteHeader(http.StatusOK)
    fmt.Fprintln(w, "OK")
}
```

#### 4.1.2 Frontend Enhancements

**File:** `examples/monitor-webui/web/static/js/team-dashboard.js` (new)

```javascript
/**
 * Team Dashboard - Active Work Panel
 */
class ActiveWorkPanel {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.refreshInterval = 5000; // 5 seconds
        this.data = [];
    }

    async refresh() {
        try {
            const response = await fetch('/api/active');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            this.data = await response.json();
            this.render();
        } catch (error) {
            console.error('Failed to fetch active work:', error);
        }
    }

    render() {
        if (!this.container) return;

        if (this.data.length === 0) {
            this.container.innerHTML = `
                <div class="empty-state">
                    <p>No active work sessions</p>
                </div>
            `;
            return;
        }

        const html = this.data.map(item => `
            <div class="active-work-item" data-id="${item.id}">
                <div class="work-header">
                    <span class="issue-id">${item.id}</span>
                    <span class="status-badge status-${item.status}">${item.status}</span>
                </div>
                <div class="work-title">${this.escapeHtml(item.title)}</div>
                <div class="work-meta">
                    <span class="session">
                        <span class="icon">👤</span>
                        ${item.agent || item.session || 'Unknown'}
                    </span>
                    <span class="duration">
                        <span class="icon">⏱️</span>
                        ${this.formatDuration(item.duration_mins)}
                    </span>
                </div>
            </div>
        `).join('');

        this.container.innerHTML = `
            <div class="panel-header">
                <h3>🔄 Active Work (${this.data.length})</h3>
                <button onclick="activeWorkPanel.refresh()" class="refresh-btn">↻</button>
            </div>
            <div class="active-work-list">
                ${html}
            </div>
        `;
    }

    formatDuration(mins) {
        if (!mins || mins <= 0) return 'just now';
        if (mins < 60) return `${mins}m ago`;
        const hours = Math.floor(mins / 60);
        const remaining = mins % 60;
        return `${hours}h ${remaining}m ago`;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    start() {
        this.refresh();
        setInterval(() => this.refresh(), this.refreshInterval);
    }
}

/**
 * Dependency Graph Visualization using D3.js
 */
class DependencyGraph {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.width = 800;
        this.height = 600;
        this.svg = null;
        this.simulation = null;
    }

    async load(rootId = null) {
        try {
            const url = rootId ? `/api/deps/graph?root=${rootId}` : '/api/deps/graph';
            const response = await fetch(url);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.render(data);
        } catch (error) {
            console.error('Failed to load dependency graph:', error);
        }
    }

    render(data) {
        if (!this.container || !data.nodes || !data.edges) return;

        // Clear existing
        this.container.innerHTML = '';

        // Create SVG
        this.svg = d3.select(this.container)
            .append('svg')
            .attr('width', this.width)
            .attr('height', this.height);

        // Create force simulation
        this.simulation = d3.forceSimulation(data.nodes)
            .force('link', d3.forceLink(data.edges).id(d => d.id).distance(100))
            .force('charge', d3.forceManyBody().strength(-300))
            .force('center', d3.forceCenter(this.width / 2, this.height / 2));

        // Draw edges
        const link = this.svg.append('g')
            .selectAll('line')
            .data(data.edges)
            .enter().append('line')
            .attr('stroke', '#999')
            .attr('stroke-opacity', 0.6)
            .attr('stroke-width', 2)
            .attr('marker-end', 'url(#arrowhead)');

        // Draw nodes
        const node = this.svg.append('g')
            .selectAll('g')
            .data(data.nodes)
            .enter().append('g')
            .call(d3.drag()
                .on('start', (event, d) => this.dragstarted(event, d))
                .on('drag', (event, d) => this.dragged(event, d))
                .on('end', (event, d) => this.dragended(event, d)));

        node.append('circle')
            .attr('r', 20)
            .attr('fill', d => this.getNodeColor(d.status));

        node.append('text')
            .attr('dy', 4)
            .attr('text-anchor', 'middle')
            .attr('font-size', '10px')
            .text(d => d.id);

        // Add arrowhead marker
        this.svg.append('defs').append('marker')
            .attr('id', 'arrowhead')
            .attr('viewBox', '-0 -5 10 10')
            .attr('refX', 25)
            .attr('refY', 0)
            .attr('orient', 'auto')
            .attr('markerWidth', 8)
            .attr('markerHeight', 8)
            .append('path')
            .attr('d', 'M 0,-5 L 10 ,0 L 0,5')
            .attr('fill', '#999');

        // Update positions on tick
        this.simulation.on('tick', () => {
            link
                .attr('x1', d => d.source.x)
                .attr('y1', d => d.source.y)
                .attr('x2', d => d.target.x)
                .attr('y2', d => d.target.y);

            node.attr('transform', d => `translate(${d.x},${d.y})`);
        });
    }

    getNodeColor(status) {
        const colors = {
            'open': '#4a90d9',
            'in_progress': '#f5a623',
            'closed': '#7ed321',
            'blocked': '#d0021b',
            'deferred': '#9b9b9b'
        };
        return colors[status] || '#4a90d9';
    }

    dragstarted(event, d) {
        if (!event.active) this.simulation.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
    }

    dragged(event, d) {
        d.fx = event.x;
        d.fy = event.y;
    }

    dragended(event, d) {
        if (!event.active) this.simulation.alphaTarget(0);
        d.fx = null;
        d.fy = null;
    }
}

// Initialize on page load
let activeWorkPanel;
let dependencyGraph;

document.addEventListener('DOMContentLoaded', () => {
    activeWorkPanel = new ActiveWorkPanel('active-work-panel');
    activeWorkPanel.start();

    dependencyGraph = new DependencyGraph('dependency-graph');
    // Load on demand via button click
});
```

#### 4.1.3 Updated HTML

**File:** `examples/monitor-webui/web/index.html` (additions)

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Beads Team Dashboard</title>
    <link rel="stylesheet" href="/static/css/styles.css">
    <link rel="stylesheet" href="/static/css/team-dashboard.css">
    <script src="https://d3js.org/d3.v7.min.js"></script>
</head>
<body>
    <header>
        <h1>🔴 Beads Team Dashboard</h1>
        <div id="sync-status" class="sync-status">
            <span class="sync-indicator"></span>
            <span class="sync-text">Synced</span>
        </div>
    </header>

    <main>
        <!-- Stats Bar -->
        <section id="stats-bar" class="stats-bar">
            <!-- Populated by JavaScript -->
        </section>

        <!-- Active Work Panel (NEW) -->
        <section id="active-work-panel" class="panel active-work-panel">
            <div class="loading">Loading active work...</div>
        </section>

        <!-- Issue List (existing) -->
        <section id="issue-list" class="panel issue-list">
            <!-- Existing issue list code -->
        </section>

        <!-- Dependency Graph (NEW) -->
        <section id="dep-graph-section" class="panel dep-graph-section">
            <div class="panel-header">
                <h3>🌳 Dependency Graph</h3>
                <button onclick="dependencyGraph.load()" class="load-btn">Load Graph</button>
            </div>
            <div id="dependency-graph" class="graph-container"></div>
        </section>
    </main>

    <script src="/static/js/app.js"></script>
    <script src="/static/js/team-dashboard.js"></script>
    <script src="/static/js/websocket.js"></script>
</body>
</html>
```

### 4.2 Deployment Architecture

#### 4.2.1 Docker Compose

**File:** `examples/monitor-webui/docker-compose.yml`

```yaml
version: '3.8'

services:
  beads-dashboard:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    volumes:
      - ${BEADS_DIR:-.beads}:/app/.beads:ro
      - /var/run/beads:/var/run/beads:ro  # For daemon socket
    environment:
      - BEADS_DB=/app/.beads/beads.db
      - BEADS_SOCKET=/var/run/beads/bd.sock
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/stats"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Optional: Reverse proxy for HTTPS
  nginx:
    image: nginx:alpine
    ports:
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
    depends_on:
      - beads-dashboard
```

#### 4.2.2 Kubernetes Deployment

**File:** `examples/monitor-webui/k8s/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beads-dashboard
  labels:
    app: beads-dashboard
spec:
  replicas: 1
  selector:
    matchLabels:
      app: beads-dashboard
  template:
    metadata:
      labels:
        app: beads-dashboard
    spec:
      containers:
      - name: dashboard
        image: beads/monitor-webui:latest
        ports:
        - containerPort: 8080
        env:
        - name: BEADS_DB
          value: /data/.beads/beads.db
        volumeMounts:
        - name: beads-data
          mountPath: /data/.beads
          readOnly: true
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: beads-data
        persistentVolumeClaim:
          claimName: beads-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: beads-dashboard
spec:
  selector:
    app: beads-dashboard
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

---

## 5. Data Flow Diagrams

### 5.1 Async Sync Flow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                           Async Sync Flow                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  1. Agent calls MCP tool                                                      │
│     │                                                                         │
│     ▼                                                                         │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ MCP Server: beads_update_issue(id="bd-f7k2", status="in_progress")      │ │
│  └───────────────────────────────────┬─────────────────────────────────────┘ │
│                                      │                                        │
│  2. Tool returns immediately         │                                        │
│     │                                │                                        │
│     ▼                                ▼                                        │
│  ┌───────────────────┐     ┌───────────────────────────────────────────────┐ │
│  │ Agent continues   │     │ PostToolUse Hook (async: true)                │ │
│  │ with next task    │     │                                               │ │
│  │                   │     │ 3. Spawn background process                   │ │
│  │ (not blocked)     │     │    │                                          │ │
│  └───────────────────┘     │    ▼                                          │ │
│                            │ ┌─────────────────────────────────────────┐   │ │
│                            │ │ beads-team-sync.sh                      │   │ │
│                            │ │                                         │   │ │
│                            │ │ 4. Acquire lock                         │   │ │
│                            │ │    │                                    │   │ │
│                            │ │    ▼                                    │   │ │
│                            │ │ 5. bd sync --status (check changes)     │   │ │
│                            │ │    │                                    │   │ │
│                            │ │    ▼                                    │   │ │
│                            │ │ 6. bd sync (export + commit + push)     │   │ │
│                            │ │    │                                    │   │ │
│                            │ │    ▼                                    │   │ │
│                            │ │ 7. Webhook notify (optional)            │   │ │
│                            │ │    │                                    │   │ │
│                            │ │    ▼                                    │   │ │
│                            │ │ 8. Release lock, exit                   │   │ │
│                            │ └─────────────────────────────────────────┘   │ │
│                            └───────────────────────────────────────────────┘ │
│                                      │                                        │
│                                      ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Remote Git Repository (GitHub/GitLab)                                   │ │
│  │ - beads_sync branch updated                                             │ │
│  │ - Team can see changes                                                  │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Dashboard Real-Time Update Flow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                       Dashboard Real-Time Update Flow                         │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌───────────────────┐                                                       │
│  │ Beads Daemon      │                                                       │
│  │                   │                                                       │
│  │ Mutation occurs:  │                                                       │
│  │ - Issue created   │                                                       │
│  │ - Status changed  │                                                       │
│  │ - Issue closed    │                                                       │
│  └─────────┬─────────┘                                                       │
│            │                                                                  │
│            │ 1. Mutation event                                               │
│            ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Monitor WebUI Backend                                                    │ │
│  │                                                                          │ │
│  │ pollMutations() goroutine:                                              │ │
│  │                                                                          │ │
│  │ 2. Call daemon RPC: GetMutations(since=lastID)                          │ │
│  │    │                                                                     │ │
│  │    ▼                                                                     │ │
│  │ 3. Receive mutation list                                                │ │
│  │    │                                                                     │ │
│  │    ▼                                                                     │ │
│  │ 4. Format as WebSocket message                                          │ │
│  │    │                                                                     │ │
│  │    ▼                                                                     │ │
│  │ 5. wsBroadcast <- message                                               │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│            │                                                                  │
│            │ 6. WebSocket broadcast                                          │
│            ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Browser (Multiple Connected Clients)                                     │ │
│  │                                                                          │ │
│  │ ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                    │ │
│  │ │ Client A     │  │ Client B     │  │ Client C     │                    │ │
│  │ │              │  │              │  │              │                    │ │
│  │ │ 7. Receive   │  │ 7. Receive   │  │ 7. Receive   │                    │ │
│  │ │    WS msg    │  │    WS msg    │  │    WS msg    │                    │ │
│  │ │              │  │              │  │              │                    │ │
│  │ │ 8. Update    │  │ 8. Update    │  │ 8. Update    │                    │ │
│  │ │    DOM       │  │    DOM       │  │    DOM       │                    │ │
│  │ └──────────────┘  └──────────────┘  └──────────────┘                    │ │
│  │                                                                          │ │
│  │ Updates visible within ~2 seconds of change                             │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. Testing Strategy

### 6.1 Unit Tests

```go
// internal/tasks/active_session_test.go

func TestActiveSessionTracking(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    // Create issue
    issue, err := store.CreateIssue(&types.Issue{
        Title:  "Test issue",
        Status: "open",
    })
    require.NoError(t, err)

    // Update to in_progress with session
    os.Setenv("CLAUDE_SESSION_ID", "test-session-123")
    defer os.Unsetenv("CLAUDE_SESSION_ID")

    err = store.UpdateIssue(issue.ID, map[string]interface{}{
        "status": "in_progress",
    })
    require.NoError(t, err)

    // Verify active session set
    updated, err := store.GetIssue(issue.ID)
    require.NoError(t, err)
    assert.Equal(t, "test-session-123", updated.ActiveSession)
    assert.NotNil(t, updated.ActiveSessionStartedAt)
}

func TestOptimisticLocking(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    // Create issue
    issue, err := store.CreateIssue(&types.Issue{
        Title:  "Test issue",
        Status: "open",
    })
    require.NoError(t, err)

    originalUpdatedAt := issue.UpdatedAt

    // Simulate concurrent modification
    time.Sleep(10 * time.Millisecond)
    err = store.UpdateIssue(issue.ID, map[string]interface{}{
        "status": "in_progress",
    })
    require.NoError(t, err)

    // Try update with stale timestamp
    err = store.UpdateIssueWithLock(issue.ID, map[string]interface{}{
        "status": "closed",
    }, originalUpdatedAt)

    assert.ErrorIs(t, err, ErrConflict)
}
```

### 6.2 Integration Tests

```bash
#!/bin/bash
# tests/integration/team_sync_test.sh

set -e

echo "=== Team Sync Integration Test ==="

# Setup test environment
TEST_DIR=$(mktemp -d)
cd "$TEST_DIR"
git init
bd init --prefix test

# Create issue
ISSUE_ID=$(bd create "Test issue" --json | jq -r '.id')
echo "Created issue: $ISSUE_ID"

# Simulate MCP tool call with hook
export CLAUDE_SESSION_ID="test-session-$(date +%s)"

# Trigger async sync hook
.claude/hooks/beads-team-sync.sh &
HOOK_PID=$!

# Wait for sync
sleep 3

# Verify sync completed
if git log --oneline -1 | grep -q "beads"; then
    echo "✓ Sync commit created"
else
    echo "✗ Sync commit not found"
    exit 1
fi

# Cleanup
kill $HOOK_PID 2>/dev/null || true
rm -rf "$TEST_DIR"

echo "=== Test Passed ==="
```

### 6.3 E2E Dashboard Tests

```javascript
// tests/e2e/dashboard.spec.js (Playwright)

const { test, expect } = require('@playwright/test');

test.describe('Team Dashboard', () => {
    test.beforeEach(async ({ page }) => {
        await page.goto('http://localhost:8080');
    });

    test('shows active work panel', async ({ page }) => {
        await expect(page.locator('#active-work-panel')).toBeVisible();
        await expect(page.locator('.panel-header h3')).toContainText('Active Work');
    });

    test('updates in real-time via WebSocket', async ({ page }) => {
        // Wait for WebSocket connection
        await page.waitForTimeout(1000);

        // Trigger update via API
        await page.evaluate(async () => {
            await fetch('/webhook', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    event: 'sync_complete',
                    timestamp: new Date().toISOString()
                })
            });
        });

        // Verify notification received
        await expect(page.locator('.sync-indicator')).toHaveClass(/synced/);
    });

    test('loads dependency graph on demand', async ({ page }) => {
        await page.click('button:has-text("Load Graph")');
        await expect(page.locator('#dependency-graph svg')).toBeVisible();
    });
});
```

---

## 7. Rollout Plan

### Phase 1: Alpha (Week 1-2)
- [ ] Implement async sync hook
- [ ] Add active session tracking to schema
- [ ] Internal testing with development team

### Phase 2: Beta (Week 3-4)
- [ ] Enhance monitor-webui with team features
- [ ] Docker deployment option
- [ ] Limited external beta with select users

### Phase 3: GA (Week 5-6)
- [ ] Documentation and tutorials
- [ ] `bd setup claude --team-sync` command
- [ ] Announce to community

### Rollback Plan

1. **Hook rollback**: Remove PostToolUse hook config
2. **Schema rollback**: New fields are additive (no breaking changes)
3. **Dashboard rollback**: Revert to previous monitor-webui

---

## 8. Appendices

### A. Configuration Reference

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `BEADS_DIR` | `$(git root)/.beads` | Beads data directory |
| `BEADS_SYNC_LOG` | `~/.beads/team-sync.log` | Sync log file |
| `BEADS_SYNC_DEBOUNCE` | `10` | **Seconds to wait after last change before git push** |
| `BEADS_SYNC_LOCK_TIMEOUT` | `30` | Lock timeout in seconds |
| `BEADS_DASHBOARD_WEBHOOK` | (none) | Webhook URL for notifications |
| `BEADS_SYNC_BRANCH` | (from config) | Git branch for sync |
| `BD_ACTOR` | (git user.name) | Actor identity for tracking who made changes |
| `BEADS_ACTOR` | (alias for BD_ACTOR) | MCP-compatible actor identity |

### B. API Reference

| Endpoint | Method | Description | Response |
|----------|--------|-------------|----------|
| `/api/team` | GET | Team members with in-progress work by GitHub username | `{members: [{github_username, in_progress_count, issues}]}` |
| `/api/deps/graph` | GET | Dependency graph | `{nodes: [], edges: []}` |
| `/api/sync/status` | GET | Sync state | `{pending_changes, last_sync_at}` |
| `/webhook` | POST | Receive sync notifications | `OK` |

**Note:** The original `/api/active` endpoint for real-time session tracking has been deferred due to the lack of reliable session identification without `CLAUDE_SESSION_ID`.

### C. Related Files

| File | Purpose |
|------|---------|
| `docs/prd/team-collaboration-prd.md` | Requirements document |
| `examples/monitor-webui/` | Dashboard source |
| `.claude/hooks/beads-team-sync.sh` | Async sync hook |
| `internal/storage/sqlite/migrations/036_*.go` | Schema migration |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-27 | System 3 | Initial solution design |
| 1.1 | 2026-01-27 | System 3 | Incorporated user feedback: 10s debounce for git push, GitHub username for ownership, deferred active_session tracking, updated API endpoints |
