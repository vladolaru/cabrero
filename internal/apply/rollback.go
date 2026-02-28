package apply

import (
	"fmt"
	"os"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/store"
)

// Rollback restores the previous content for a change entry.
// Returns the file path that was restored, or an error.
func Rollback(changeID string) (string, error) {
	entry, err := store.GetChange(changeID)
	if err != nil {
		return "", fmt.Errorf("reading change: %w", err)
	}
	if entry == nil {
		return "", fmt.Errorf("change %q not found", changeID)
	}
	if entry.PreviousContent == "" {
		return "", fmt.Errorf("change %q has no previous content (cannot rollback)", changeID)
	}
	if entry.Status == "rollback" {
		return "", fmt.Errorf("change %q is already a rollback entry", changeID)
	}

	if err := os.WriteFile(entry.FilePath, []byte(entry.PreviousContent), 0o644); err != nil {
		return "", fmt.Errorf("writing rollback: %w", err)
	}

	// Append audit trail.
	auditEntry := fitness.ChangeEntry{
		ID:           "rollback-" + changeID,
		SourceName:   entry.SourceName,
		SourceOrigin: entry.SourceOrigin,
		ProposalID:   entry.ProposalID,
		Description:  "Rollback of " + changeID,
		Timestamp:    time.Now(),
		Status:       "rollback",
		FilePath:     entry.FilePath,
	}
	if err := store.AppendChange(auditEntry); err != nil {
		return entry.FilePath, fmt.Errorf("rollback succeeded but audit trail failed: %w", err)
	}

	return entry.FilePath, nil
}
