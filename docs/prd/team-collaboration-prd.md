# PRD: Beads Team Collaboration Platform

**Version:** 1.1
**Status:** Final Draft
**Author:** System 3 Meta-Orchestrator
**Date:** 2026-01-27
**Updated:** 2026-01-27 (incorporated user feedback)

---

## Executive Summary

This PRD defines the evolution of Beads from a single-engineer local tool to a **team collaboration platform**. It covers two primary goals:

1. **Goal #1: Continuous Sync** - Real-time synchronization of beads epics, tasks, and status across team members to prevent duplicate work and enable multi-agent coordination.

2. **Goal #2: Browser Interface** - A web-based dashboard for viewing and managing beads on the synchronized branch, enabling non-CLI users and providing team-wide visibility.

**Strategic Vision:** Future integration with Linear, Slack, and Atlassian JIRA to position Beads as a universal bridge between AI coding agents and enterprise project management tools.

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals and Non-Goals](#2-goals-and-non-goals)
3. [Background Research](#3-background-research)
4. [Goal #1: Continuous Sync Architecture](#4-goal-1-continuous-sync-architecture)
5. [Goal #2: Browser Interface](#5-goal-2-browser-interface)
6. [Async Hooks Integration](#6-async-hooks-integration)
7. [Data Model Extensions](#7-data-model-extensions)
8. [Security Considerations](#8-security-considerations)
9. [Implementation Phases](#9-implementation-phases)
10. [Future: Enterprise Integrations](#10-future-enterprise-integrations)
11. [Open Questions](#11-open-questions)
12. [Appendix](#appendix)

---

## 1. Problem Statement

### Current State

Beads currently operates as a **local-first, single-engineer tool**:

- **Local SQLite database** (`.beads/beads.db`) for fast queries
- **JSONL export** (`.beads/issues.jsonl`) for git-based distribution
- **Manual sync** via `bd sync` command
- **Daemon auto-export** with 5-second debounce
- **Sync branch** feature for isolated beads history

### Gap Analysis

| Capability | Current | Required for Teams |
|------------|---------|-------------------|
| Real-time sync | Manual `bd sync` | Automatic push on MCP tool use |
| Conflict resolution | 3-way merge on pull | Real-time conflict prevention |
| Team awareness | None | Live status visibility |
| Web interface | CLI only | Browser dashboard |
| Multi-agent coordination | Limited | Full visibility + locking |
| External integrations | None | Linear, Slack, JIRA |

### User Stories

> "As a team lead, I want to see what issues my team members are working on in real-time, so I can coordinate work and prevent duplicate effort."

> "As a remote team member, I want a web dashboard to view beads status without needing CLI access, so I can stay informed during meetings."

> "As a System 3 meta-orchestrator, I want automatic sync after each beads operation, so my spawned orchestrators have consistent state."

> "As a team using Linear/JIRA, I want bi-directional sync with beads, so AI agents can work within our existing workflow."

---

## 2. Goals and Non-Goals

### Goal #1: Continuous Sync

**Goals:**
- ✅ Automatic git sync after each beads MCP tool call (create, update, close)
- ✅ Leverage Claude Code async hooks (non-blocking background execution)
- ✅ Prevent concurrent edits to same issue across team members
- ✅ Sub-second latency for local operations, background sync

**Non-Goals (V1):**
- ❌ Real-time push notifications (polling is sufficient for V1)
- ❌ Offline-first with complex conflict resolution
- ❌ P2P sync without central git repository

### Goal #2: Browser Interface

**Goals:**
- ✅ Real-time dashboard showing all team beads status
- ✅ Filter by status, priority, assignee, labels
- ✅ Dependency graph visualization
- ✅ Epic/subtask hierarchy view
- ✅ Integration with existing `monitor-webui` architecture

**Non-Goals (V1):**
- ❌ Full CRUD operations from web (CLI/MCP remains primary)
- ❌ User authentication/authorization (team-internal use)
- ❌ Mobile-native apps

---

## 3. Background Research

### 3.1 Existing Beads Ecosystem

#### Core Project Components

| Component | Location | Purpose |
|-----------|----------|---------|
| **Monitor WebUI** | `examples/monitor-webui/` | Production-ready web dashboard |
| **Daemon Architecture** | `cmd/bd/daemon*.go` | Event-driven auto-sync |
| **Sync Branch** | `internal/syncbranch/` | Isolated beads history via worktrees |
| **MCP Server** | `integrations/beads-mcp/` | AI agent interface |
| **Task Tracking PRD** | `docs/prd/task-tracking-sync.md` | Claude Code ↔ beads sync |

#### Community Web UIs

| Project | Tech Stack | Key Features | Status |
|---------|------------|--------------|--------|
| **beads-ui** (mantoni) | Node.js | Live updates, kanban | Active |
| **beads-dashboard** (rhydlewis) | Node.js/React | Metrics, lead time | Active |
| **beads-kanban-ui** (AvivK5498) | TypeScript/Rust | Kanban, git branch tracking | Active |
| **Monitor WebUI** (core) | Go + vanilla JS | Real-time, WebSocket | Stable |

**Recommendation:** Extend `monitor-webui` as the canonical team dashboard, leveraging existing daemon RPC integration.

### 3.2 Claude Code Async Hooks (January 2026)

**New capability:** Hooks can now run in the background without blocking Claude Code execution.

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "mcp__beads*",
      "hooks": [{
        "type": "command",
        "command": "./beads-sync.sh",
        "async": true,
        "timeout": 30
      }]
    }]
  }
}
```

**Key benefits:**
- Git push after each beads operation without blocking agent
- Background notifications to team dashboards
- Decoupled sync from tool execution latency

### 3.3 Existing Sync Architecture

The beads daemon uses an **event-driven architecture** with:

1. **Mutation channel** (512 buffer) → Debouncer (500ms) → Export → Git commit/push
2. **File watcher** (fsnotify) → Import debouncer (500ms) → Git pull → Import
3. **Remote sync** polling (30s default) → Git pull → Import

**Extension point:** Add async hook trigger after MCP tool calls.

### 3.4 Integration Patterns Research

| System | API Type | Sync Pattern | Webhook Support |
|--------|----------|--------------|-----------------|
| **Linear** | GraphQL | Event-driven webhooks | Yes |
| **JIRA** | REST | Polling or webhooks | Yes |
| **Slack** | REST | Incoming webhooks | Yes |
| **GitHub Issues** | GraphQL | Webhooks | Yes |

**Community tool:** `jira-beads-sync` (conallob) provides reference implementation.

---

## 4. Goal #1: Continuous Sync Architecture

### 4.1 Design Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Claude Code Session (Agent A)                        │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ MCP Tool Call: beads_update_issue(id="bd-f7k2", status="done")    │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                                     │
                                     │ PostToolUse hook (async: true)
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    Async Sync Hook (beads-sync.sh)                      │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ 1. Check for pending changes (bd sync --status)                   │  │
│  │ 2. If changes: bd sync (export + commit + push)                   │  │
│  │ 3. Log to ~/.beads/sync.log                                       │  │
│  │ 4. Optional: Notify dashboard via webhook                         │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                                     │
                                     │ git push to sync branch
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    Central Git Repository                               │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ Branch: beads_sync                                                │  │
│  │ File: .beads/issues.jsonl                                         │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                      │
              ▼                      ▼                      ▼
┌─────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐
│   Agent B (pulls)   │ │   Agent C (polls)   │ │  Web Dashboard      │
│   Remote sync: 30s  │ │   Remote sync: 30s  │ │  Webhook notify     │
└─────────────────────┘ └─────────────────────┘ └─────────────────────┘
```

### 4.2 Async Hook Implementation

**Key Design Decisions (from user feedback):**

1. **Hook Filtering**: The PostToolUse hook only fires when beads MCP tools are called (via matcher pattern). Non-beads tool calls do NOT trigger the sync hook.

2. **10-Second Debounce**: To avoid flooding GitHub with rapid commits/pushes during active editing, the hook implements a 10-second quiet period. After receiving a change, it waits 10 seconds for additional changes before committing and pushing. This batches rapid updates into single commits.

3. **Local Queue System**: The debounce is implemented via a timestamp file. Each hook invocation updates the "last change" timestamp. Only when 10 seconds have passed since the last change does the sync execute.

**File:** `.claude/hooks/beads-sync-hook.sh`

```bash
#!/bin/bash
# Async hook for beads sync after MCP tool calls
# Triggered by PostToolUse with matcher "mcp__beads*"
#
# IMPORTANT: This hook ONLY runs when beads tools are called.
# The matcher pattern "mcp__beads*|mcp__plugin_beads*" ensures
# non-beads MCP tool calls do NOT trigger this hook.
#
# DEBOUNCE: Waits 10 seconds after last change before sync
# to avoid flooding GitHub with rapid commits.

set -e

BEADS_DIR="${BEADS_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)/.beads}"
SYNC_LOG="${HOME}/.beads/sync.log"
LOCK_FILE="${BEADS_DIR}/.sync.lock"
DEBOUNCE_FILE="${BEADS_DIR}/.sync-debounce"
DEBOUNCE_SECONDS="${BEADS_SYNC_DEBOUNCE:-10}"

log() {
    echo "$(date -u +%FT%TZ) [$$] $*" >> "$SYNC_LOG"
}

# Record this change timestamp for debounce
echo "$(date +%s)" > "$DEBOUNCE_FILE"
log "Change detected, debounce timer reset"

# Avoid concurrent syncs
if [ -f "$LOCK_FILE" ]; then
    pid=$(cat "$LOCK_FILE" 2>/dev/null)
    if kill -0 "$pid" 2>/dev/null; then
        log "Sync already in progress (PID: $pid), exiting"
        exit 0
    fi
fi

# Wait for debounce period (10 seconds of quiet)
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
echo $$ > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT

# Check if sync is needed
if ! bd sync --status 2>/dev/null | grep -q "changes pending"; then
    log "No changes pending, skipping sync"
    exit 0
fi

# Perform sync
log "Starting async sync..."
if bd sync 2>&1 | tee -a "$SYNC_LOG"; then
    log "Sync completed successfully"

    # Optional: Notify dashboard via webhook
    if [ -n "$BEADS_DASHBOARD_WEBHOOK" ]; then
        curl -s -X POST "$BEADS_DASHBOARD_WEBHOOK" \
            -H "Content-Type: application/json" \
            -d '{"event":"sync_complete","timestamp":"'$(date -u +%FT%TZ)'"}' &
    fi
else
    log "Sync failed: $?"
fi
```

**Hook configuration** (`.claude/settings.json`):

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__beads*|mcp__plugin_beads*",
        "hooks": [{
          "type": "command",
          "command": ".claude/hooks/beads-sync-hook.sh",
          "async": true,
          "timeout": 60
        }]
      }
    ]
  }
}
```

### 4.3 Conflict Prevention: Optimistic Locking

To prevent concurrent edits, add optimistic locking to the MCP server:

**Strategy:** Include `updated_at` timestamp in update requests; reject if stale.

```python
# integrations/beads-mcp/src/beads_mcp/tools.py

async def beads_update_issue(
    issue_id: str,
    expected_updated_at: str | None = None,  # NEW: Optimistic lock
    **kwargs
) -> dict:
    """Update issue with optimistic locking."""

    # Fetch current state
    current = await client.show(issue_id)

    # Check for concurrent modification
    if expected_updated_at:
        if current.updated_at.isoformat() != expected_updated_at:
            return {
                "error": "CONFLICT",
                "message": f"Issue {issue_id} was modified by another session",
                "current_updated_at": current.updated_at.isoformat(),
                "expected_updated_at": expected_updated_at,
                "hint": "Refresh issue state before retrying"
            }

    # Proceed with update
    return await client.update(issue_id, **kwargs)
```

### 4.4 Team Awareness: Active Work Visibility

**Key Design Decisions (from research and user feedback):**

1. **Actor-Based Identification**: Instead of relying on `CLAUDE_SESSION_ID` (which may not be set), we use the existing **Actor** field that beads already tracks. The Actor is resolved via priority chain:
   - `--actor` CLI flag (explicit override)
   - `BD_ACTOR` environment variable
   - `BEADS_ACTOR` environment variable (MCP compatibility)
   - Git config `user.name`
   - `USER` environment variable
   - `"unknown"` fallback

2. **GitHub Username for Ownership**: Add `github_username` field so issues can be *owned* by a GitHub identity. This enables:
   - Clear ownership in team dashboard
   - Integration with GitHub Issues sync
   - API endpoints like `/api/team` using GH username

3. **No Active Session Tracking (Deferred)**: The original `active_session` field concept is deferred. Without reliable session identification, we cannot track "who is actively working on what" in real-time. The **Actor** field provides sufficient "who last touched this" tracking.

**New fields in Issue:**

```sql
-- internal/storage/sqlite/migrations/036_team_collaboration.go
-- Ownership tracking
ALTER TABLE issues ADD COLUMN github_username TEXT;

-- Sync metadata
ALTER TABLE issues ADD COLUMN last_synced_at TIMESTAMP;
ALTER TABLE issues ADD COLUMN sync_source TEXT;  -- 'local', 'remote:<machine>'

-- Team grouping (optional)
ALTER TABLE issues ADD COLUMN team TEXT;

-- Indexes
CREATE INDEX idx_issues_github_username ON issues(github_username) WHERE github_username IS NOT NULL;
CREATE INDEX idx_issues_team ON issues(team) WHERE team IS NOT NULL;
```

**CLI support:**

```bash
# Set GitHub username for ownership (one-time setup)
bd config set user.github-username alice

# Create issue with ownership
bd create "Implement auth" --type task
# Automatically sets github_username from config

# Claim issue (update actor tracking)
BD_ACTOR="alice" bd update bd-f7k2 --status in_progress

# List who's working on what (by GitHub username)
bd list --status=in_progress --by-owner
# Output:
# alice (3 issues):
#   bd-f7k2  Implement auth      [in_progress]  P1
#   bd-x9y3  Fix payment bug     [in_progress]  P2
# bob (1 issue):
#   bd-m0f   MCP Skills Plugin   [in_progress]  P1

# View issues by GitHub username
bd list --github-username=alice
```

**API Endpoint Update:**

```
GET /api/team
Response:
{
  "members": [
    {
      "github_username": "alice",
      "in_progress_count": 3,
      "total_owned": 15,
      "issues": ["bd-f7k2", "bd-x9y3", "bd-m0f"]
    },
    {
      "github_username": "bob",
      "in_progress_count": 1,
      "total_owned": 8,
      "issues": ["bd-abc"]
    }
  ]
}
```

---

## 5. Goal #2: Browser Interface

### 5.1 Architecture Decision

**Recommendation:** Extend `examples/monitor-webui/` rather than building new.

**Rationale:**
- Already production-ready with WebSocket support
- Connects to daemon via RPC (avoids DB locking)
- Clean separation: Go backend + vanilla JS frontend
- Easily extensible for team features

### 5.2 Enhanced Monitor WebUI Features

#### 5.2.1 Team Dashboard View

```
┌─────────────────────────────────────────────────────────────────────────┐
│  🔴 Beads Team Dashboard                                    [Refresh]   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  📊 Stats: 38 Open | 6 In Progress | 32 Ready | 600 Closed             │
│                                                                         │
│  🔄 Active Work (6)                                                     │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ bd-f7k2  Implement auth      ● System3-Auth  Started: 45m ago   │   │
│  │ bd-x9y3  Fix payment bug     ● Human:alice   Started: 2h ago    │   │
│  │ bd-m0f   MCP Skills Plugin   ○ Unassigned    Ready to work      │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  📋 Issue List (filtered)                                   [Filters ▼]│
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ ID       │ Title                │ Status     │ Priority │ Type  │   │
│  │──────────┼──────────────────────┼────────────┼──────────┼───────│   │
│  │ bd-3x6   │ MCP Skills Plugin    │ open       │ P1       │ epic  │   │
│  │ bd-d1n   │ Gate-Aware Release   │ open       │ P1       │ epic  │   │
│  │ bd-8y2   │ Melbourne Campaign   │ open       │ P2       │ task  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  🌳 Dependency Graph                                        [Expand]    │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │         [bd-cf2 BO Marketplace]                                 │   │
│  │              ├── bd-4z1 Plugin structure                        │   │
│  │              ├── bd-4ol Skill bundling                          │   │
│  │              ├── bd-m0f Session hook                            │   │
│  │              ├── bd-cvq Local testing                           │   │
│  │              └── bd-9a3 Submit to marketplace                   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### 5.2.2 New API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/active` | GET | Active work sessions with agent info |
| `/api/team` | GET | Team members and their current work |
| `/api/deps/graph` | GET | Full dependency graph as JSON |
| `/api/sync/status` | GET | Last sync time, pending changes |
| `/api/webhook` | POST | Receive sync notifications |

#### 5.2.3 Real-Time Updates via WebSocket

**How WebSocket Broadcasting Works (from codebase research):**

The existing Monitor WebUI already implements WebSocket broadcasting:

1. **Daemon emits mutations** via `emitMutation()` or `emitRichMutation()` to an internal channel (buffer=512)
2. **Monitor WebUI polls daemon** every 2 seconds via RPC `GetMutations(since=lastTimestamp)`
3. **Mutations are broadcast** to all connected WebSocket clients via `wsBroadcast` channel (buffer=256)
4. **Clients receive updates** as JSON and update DOM in real-time

**Mutation Event Structure** (existing):

```go
type MutationEvent struct {
    Type      string    // "create", "update", "delete", "status", "sync"
    IssueID   string    // e.g., "bd-42"
    Title     string    // For display context
    Assignee  string    // Current assignee
    Actor     string    // Who performed the action (from BD_ACTOR)
    Timestamp time.Time
    OldStatus string    // Previous status
    NewStatus string    // New status
    ParentID  string    // Parent molecule (for bonded events)
}
```

**Sync Notification Message** (via webhook or direct broadcast):

```json
{
  "type": "mutation",
  "data": {
    "Type": "sync",
    "IssueID": "bd-f7k2",
    "Title": "Implement auth feature",
    "Actor": "alice",
    "Timestamp": "2026-01-27T10:30:00Z",
    "OldStatus": "open",
    "NewStatus": "in_progress"
  }
}
```

### 5.3 Deployment Options

| Option | Description | Use Case |
|--------|-------------|----------|
| **Local (default)** | Run on developer machine | Personal dashboard |
| **Shared server** | Deploy on team server | Team visibility |
| **Cloud hosted** | Deploy on cloud (Vercel, etc.) | Distributed teams |

**Docker support:**

```dockerfile
# examples/monitor-webui/Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o monitor-webui .

FROM alpine:latest
COPY --from=builder /app/monitor-webui /usr/local/bin/
COPY --from=builder /app/web /web
EXPOSE 8080
CMD ["monitor-webui", "-host", "0.0.0.0"]
```

---

## 6. Async Hooks Integration

### 6.1 Hook Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Claude Code                                      │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                    MCP Tool Execution                             │  │
│  │                                                                   │  │
│  │  1. Agent calls: beads_create_issue(...)                          │  │
│  │  2. MCP server processes request                                  │  │
│  │  3. Tool returns result to agent                                  │  │
│  │                                                                   │  │
│  │  ─────────── AGENT CONTINUES (not blocked) ───────────────────    │  │
│  │                                                                   │  │
│  │  4. PostToolUse hook fires (async: true)                          │  │
│  │  5. Background: beads-sync-hook.sh                                │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                                     │
                                     │ Parallel execution
                                     ▼
┌──────────────────────────────┐  ┌──────────────────────────────┐
│  Agent continues working...  │  │  Background sync process     │
│                              │  │  - bd sync                   │
│  Next tool call...           │  │  - git commit + push         │
│  Next reasoning step...      │  │  - webhook notify            │
└──────────────────────────────┘  └──────────────────────────────┘
```

### 6.2 Hook Configuration

**Complete `.claude/hooks/config.json`:**

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__beads*|mcp__plugin_beads*|mcp__beads_dev*",
        "hooks": [{
          "type": "command",
          "command": ".claude/hooks/beads-sync-hook.sh",
          "async": true,
          "timeout": 60
        }]
      }
    ],
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [{
          "type": "command",
          "command": "bd prime"
        }]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [{
          "type": "command",
          "command": "bd sync && echo 'Session sync complete'"
        }]
      }
    ]
  }
}
```

### 6.3 Sync Strategies

| Strategy | When to Use | Implementation |
|----------|-------------|----------------|
| **Eager** | Every MCP tool call | Async hook on all beads tools |
| **Debounced** | High-frequency updates | Daemon handles (existing) |
| **On-demand** | Manual control | `bd sync` command |
| **Session-end** | Ensure final state | SessionEnd hook |

**Recommendation:** Use **Eager** strategy with async hooks for team collaboration.

---

## 7. Data Model Extensions

### 7.1 New Fields for Team Collaboration

```sql
-- Migration: 036_team_collaboration.go

-- Ownership tracking (GitHub username for team visibility)
ALTER TABLE issues ADD COLUMN github_username TEXT;

-- Sync metadata
ALTER TABLE issues ADD COLUMN last_synced_at TIMESTAMP;
ALTER TABLE issues ADD COLUMN sync_source TEXT;  -- 'local', 'remote:<machine>'

-- Team assignment (optional grouping)
ALTER TABLE issues ADD COLUMN team TEXT;

-- Performance indexes
CREATE INDEX idx_issues_github_username ON issues(github_username)
    WHERE github_username IS NOT NULL;
CREATE INDEX idx_issues_team ON issues(team)
    WHERE team IS NOT NULL;
CREATE INDEX idx_issues_status_priority ON issues(status, priority)
    WHERE status IN ('open', 'in_progress');
```

**Note:** The original `active_session`, `active_session_started_at`, and `active_session_agent` fields have been **deferred** because we cannot reliably identify which Claude session is making changes. The Actor field (tracked via `BD_ACTOR` env var) provides sufficient "who last touched this" tracking.

### 7.2 JSONL Export Extensions

Include new fields in JSONL for distributed sync:

```json
{
  "id": "bd-f7k2",
  "title": "Implement auth feature",
  "status": "in_progress",
  "github_username": "alice",
  "last_synced_at": "2026-01-27T10:35:00Z",
  "sync_source": "local",
  "team": "backend"
}
```

### 7.3 Conflict Resolution Rules

| Field | Conflict Rule | Rationale |
|-------|---------------|-----------|
| `status` | Last-write-wins (LWW) by `updated_at` | Status changes are atomic |
| `active_session` | First-write-wins | Prevents double-claiming |
| `labels` | Union merge | Labels are additive |
| `dependencies` | Union merge | Dependencies are additive |
| `comments` | Append, dedup by ID | Comments are append-only |

---

## 8. Security Considerations

### 8.1 Authentication & Authorization (V2)

V1 assumes team-internal use with shared git access. V2 will add:

| Layer | V1 (Team) | V2 (Enterprise) |
|-------|-----------|-----------------|
| Git access | SSH keys / tokens | OAuth, SAML |
| Web dashboard | None | JWT-based auth |
| API access | Localhost only | API keys |
| Audit | Git history | Structured audit log |

### 8.2 Data Privacy

- **Issue content**: May contain sensitive code, paths, business logic
- **Session IDs**: Reveal agent/user activity patterns
- **Sync logs**: Contain timing and operational data

**Recommendation:** Keep web dashboard on private network or VPN for V1.

### 8.3 Sync Integrity

- **Hash verification**: JSONL content hash ensures data integrity
- **Git history**: Full audit trail of all changes
- **Optimistic locking**: Prevents silent overwrites

---

## 9. Implementation Phases

### Phase 1: Async Sync Hooks (1-2 weeks)

**Deliverables:**
- [ ] `beads-sync-hook.sh` script
- [ ] Hook configuration for beads MCP tools
- [ ] `bd sync --status` enhancement
- [ ] Documentation for hook setup

**Acceptance Criteria:**
- Agent calls `beads_create_issue` → background sync runs
- Sync completes without blocking agent
- Sync log captures all operations

### Phase 2: Enhanced Monitor WebUI (2-3 weeks)

**Deliverables:**
- [ ] `/api/active` endpoint for active work
- [ ] `/api/deps/graph` endpoint for dependency visualization
- [ ] Frontend: Active work panel
- [ ] Frontend: Dependency graph view (D3.js or vis.js)
- [ ] WebSocket enhancements for real-time updates

**Acceptance Criteria:**
- Dashboard shows active sessions in real-time
- Dependency graph renders correctly
- WebSocket updates within 2 seconds of change

### Phase 3: Team Collaboration Features (2-3 weeks)

**Deliverables:**
- [ ] Active session tracking fields in Issue
- [ ] Optimistic locking in MCP server
- [ ] `bd active` command
- [ ] Team filtering in dashboard

**Acceptance Criteria:**
- `bd update` sets `active_session` automatically
- Concurrent update to same issue returns CONFLICT
- Dashboard shows all team members' active work

### Phase 4: Production Hardening (1-2 weeks)

**Deliverables:**
- [ ] Docker deployment
- [ ] Webhook notification support
- [ ] Performance optimization
- [ ] Error handling and retry logic

**Acceptance Criteria:**
- Dashboard deploys via Docker Compose
- Webhook notifications work with 3rd party services
- Handles 1000+ issues without performance degradation

---

## 10. Future: Enterprise Integrations

### 10.1 Integration Architecture (V2+)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Beads Federation Hub                             │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                    Adapter Registry                               │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐          │  │
│  │  │  Linear  │  │   JIRA   │  │  Slack   │  │  GitHub  │          │  │
│  │  │ Adapter  │  │ Adapter  │  │ Adapter  │  │ Adapter  │          │  │
│  │  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘          │  │
│  │       │             │             │             │                 │  │
│  │       └─────────────┴──────┬──────┴─────────────┘                 │  │
│  │                            │                                      │  │
│  │                   ┌────────┴────────┐                             │  │
│  │                   │  Sync Engine    │                             │  │
│  │                   │  - Bidirectional│                             │  │
│  │                   │  - Conflict res │                             │  │
│  │                   │  - Field mapping│                             │  │
│  │                   └────────┬────────┘                             │  │
│  └───────────────────────────┬───────────────────────────────────────┘  │
│                              │                                          │
│                       ┌──────┴──────┐                                   │
│                       │ Beads Core  │                                   │
│                       └─────────────┘                                   │
└─────────────────────────────────────────────────────────────────────────┘
```

### 10.2 Linear Integration

**Use case:** Sync beads with Linear for product management visibility.

**Mapping:**
| Beads | Linear |
|-------|--------|
| Issue | Issue |
| Epic | Project/Cycle |
| Priority (0-4) | Priority (1-4) |
| Labels | Labels |
| Status | State |

**API pattern:** GraphQL + webhooks for real-time sync.

### 10.3 JIRA Integration

**Use case:** Enterprise teams using JIRA for official tracking.

**Reference implementation:** `jira-beads-sync` by @conallob

**Mapping:**
| Beads | JIRA |
|-------|------|
| Issue | Issue |
| Epic | Epic |
| Priority | Priority |
| Labels | Labels |
| Comments | Comments |

### 10.4 Slack Integration

**Use case:** Team notifications and awareness.

**Features:**
- New issue created → Slack notification
- Issue claimed → Team awareness
- Issue completed → Celebration message
- Daily digest of open work

---

## 11. Decisions Made (Previously Open Questions)

### Q1: Sync Branch Strategy ✅ DECIDED

**Decision:** Use **dedicated sync branch** (Option 1)

**Rationale:** Clean separation from code, prevents beads commits from cluttering feature branches.

### Q2: Web Dashboard Authentication ✅ DECIDED

**Decision:** **No auth** for V1 (localhost/VPN only)

**Rationale:** Team-internal use case. Authentication adds complexity without matching V1 use case. OAuth can be added in V2 for distributed teams.

### Q3: Base UI to Extend ✅ DECIDED

**Decision:** Extend **Monitor WebUI** (core project)

**Rationale:** Already production-ready with WebSocket support, daemon RPC integration, and clean Go backend. No need to evaluate external community UIs.

### Q4: Integration Priority ✅ DECIDED

**Decision:** **GitHub Issues** as first external integration target

**Rationale:** Most common team workflow, well-documented API, bidirectional sync reference in `jira-beads-sync`.

## 12. Remaining Open Questions

### Q5: Active Session Tracking (Deferred)

**Question:** How can we identify which Claude agent/session is actively working on an issue?

**Context:** `CLAUDE_SESSION_ID` is not reliably set. The daemon could set `BEADS_DAEMON_ID` but that doesn't identify which agent session is making changes.

**Current State:** Using Actor field (`BD_ACTOR` env var) for "who last touched" tracking. Real-time "who is actively working" tracking is **deferred** until a reliable identification mechanism exists.

**Potential Solutions to Explore:**
1. Claude Code enhancement to always set `CLAUDE_SESSION_ID`
2. MCP protocol enhancement to pass session context
3. Worktree-based identification (one worktree per orchestrator)

### Q6: Mobile Support

**Decision Pending:** Responsive web (Option 1) is sufficient for V1. PWA consideration for V2.

---

## Appendix

### A. Existing Community Tools Summary

| Tool | Type | Features | Recommendation |
|------|------|----------|----------------|
| beads-ui | Web | Live updates, kanban | Study for patterns |
| beads-dashboard | Web | Metrics, React | Study for charts |
| beads-kanban-ui | Web | TypeScript/Rust | Study for performance |
| Monitor WebUI | Web | Core, WebSocket | **Extend this** |
| beads_viewer | TUI | Go, keyboard-driven | Parallel option |
| jira-beads-sync | CLI | Go, bidirectional | Reference for integrations |

### B. Related PRDs

- [Task Tracking Sync PRD](task-tracking-sync.md): Claude Code ↔ beads sync
- Future: Linear Integration PRD
- Future: JIRA Integration PRD

### C. Technical References

- [Claude Code Hooks Documentation](https://docs.anthropic.com/claude-code/hooks)
- [Beads Sync Architecture](../SYNC.md)
- [Beads Daemon Documentation](../DAEMON.md)
- [Monitor WebUI Source](../../examples/monitor-webui/)

### D. Glossary

| Term | Definition |
|------|------------|
| **Async hook** | Hook that runs in background without blocking |
| **Optimistic locking** | Conflict detection via timestamp comparison |
| **Sync branch** | Dedicated git branch for beads history |
| **MCP** | Model Context Protocol (AI agent interface) |
| **JSONL** | JSON Lines format for git-friendly storage |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-27 | System 3 | Initial PRD with dual goals |
| 1.1 | 2026-01-27 | System 3 | Incorporated user feedback: 10s debounce, GitHub username ownership, Actor-based tracking, deferred active_session |

---

**Decisions Confirmed:**
- ✅ Sync Branch: Dedicated sync branch
- ✅ Dashboard Auth: No auth (localhost/VPN only)
- ✅ Base UI: Monitor WebUI (core project)
- ✅ Integration Priority: GitHub Issues

**Next Steps:**
1. ~~Review with stakeholder~~ ✅ Complete
2. ~~Finalize open questions~~ ✅ 4 of 6 decided
3. ~~Create Solution Design document~~ ✅ Complete
4. Begin Phase 1 implementation
