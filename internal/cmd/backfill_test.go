package cmd

import (
	"testing"
	"time"
)

func TestBuildBackfillFilter(t *testing.T) {
	t.Run("explicit since and until", func(t *testing.T) {
		filter, err := buildBackfillFilter("2025-01-15", "2025-02-15", "", false)
		if err != nil {
			t.Fatal(err)
		}
		wantSince := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
		if !filter.Since.Equal(wantSince) {
			t.Errorf("Since = %v, want %v", filter.Since, wantSince)
		}
		wantUntil := time.Date(2025, 2, 15, 23, 59, 59, 0, time.UTC)
		if !filter.Until.Equal(wantUntil) {
			t.Errorf("Until = %v, want %v", filter.Until, wantUntil)
		}
	})

	t.Run("until is expanded to end of day", func(t *testing.T) {
		filter, err := buildBackfillFilter("2025-06-01", "2025-06-01", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if filter.Until.Hour() != 23 || filter.Until.Minute() != 59 || filter.Until.Second() != 59 {
			t.Errorf("Until time = %s, want 23:59:59", filter.Until.Format("15:04:05"))
		}
	})

	t.Run("default since is approximately 30 days ago", func(t *testing.T) {
		before := time.Now().AddDate(0, 0, -30)
		filter, err := buildBackfillFilter("", "", "", false)
		after := time.Now().AddDate(0, 0, -30)
		if err != nil {
			t.Fatal(err)
		}
		if filter.Since.Before(before) || filter.Since.After(after) {
			t.Errorf("Since = %v, not in [%v, %v]", filter.Since, before, after)
		}
	})

	t.Run("empty until leaves Until as zero", func(t *testing.T) {
		filter, err := buildBackfillFilter("2025-01-01", "", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !filter.Until.IsZero() {
			t.Errorf("Until = %v, want zero", filter.Until)
		}
	})

	t.Run("default statuses is pending only", func(t *testing.T) {
		filter, err := buildBackfillFilter("", "", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if len(filter.Statuses) != 1 || filter.Statuses[0] != "pending" {
			t.Errorf("Statuses = %v, want [pending]", filter.Statuses)
		}
	})

	t.Run("retryErrors appends error status", func(t *testing.T) {
		filter, err := buildBackfillFilter("", "", "", true)
		if err != nil {
			t.Fatal(err)
		}
		if len(filter.Statuses) != 2 {
			t.Fatalf("Statuses len = %d, want 2", len(filter.Statuses))
		}
		if filter.Statuses[0] != "pending" || filter.Statuses[1] != "error" {
			t.Errorf("Statuses = %v, want [pending, error]", filter.Statuses)
		}
	})

	t.Run("project filter is passed through", func(t *testing.T) {
		filter, err := buildBackfillFilter("", "", "woocommerce", false)
		if err != nil {
			t.Fatal(err)
		}
		if filter.Project != "woocommerce" {
			t.Errorf("Project = %q, want %q", filter.Project, "woocommerce")
		}
	})

	t.Run("invalid since returns error", func(t *testing.T) {
		_, err := buildBackfillFilter("not-a-date", "", "", false)
		if err == nil {
			t.Error("expected error for invalid since")
		}
	})

	t.Run("invalid until returns error", func(t *testing.T) {
		_, err := buildBackfillFilter("2025-01-01", "bad-date", "", false)
		if err == nil {
			t.Error("expected error for invalid until")
		}
	})
}
