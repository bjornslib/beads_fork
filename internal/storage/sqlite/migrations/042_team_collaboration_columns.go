package migrations

import (
	"database/sql"
	"fmt"
)

// MigrateTeamCollaborationColumns adds columns for team collaboration:
// - github_username: GitHub username for ownership/team dashboard display
// - team: Optional team grouping
// - last_synced_at: When issue was last synced
// - sync_source: Where the sync came from ('local' or 'remote:<machine>')
func MigrateTeamCollaborationColumns(db *sql.DB) error {
	columns := []struct {
		name string
		def  string
	}{
		{"github_username", "TEXT DEFAULT ''"},
		{"team", "TEXT DEFAULT ''"},
		{"last_synced_at", "TIMESTAMP"},
		{"sync_source", "TEXT DEFAULT ''"},
	}

	for _, col := range columns {
		var exists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM pragma_table_info('issues')
			WHERE name = ?
		`, col.name).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check %s column: %w", col.name, err)
		}

		if exists {
			continue
		}

		_, err = db.Exec(fmt.Sprintf("ALTER TABLE issues ADD COLUMN %s %s", col.name, col.def))
		if err != nil {
			return fmt.Errorf("failed to add %s column: %w", col.name, err)
		}
	}

	// Add index for github_username lookups
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_issues_github_username
		ON issues(github_username)
		WHERE github_username != ''
	`)
	if err != nil {
		return fmt.Errorf("failed to create github_username index: %w", err)
	}

	// Add index for team lookups
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_issues_team
		ON issues(team)
		WHERE team != ''
	`)
	if err != nil {
		return fmt.Errorf("failed to create team index: %w", err)
	}

	return nil
}
