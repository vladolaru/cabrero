package shared

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Navigation != "arrows" {
		t.Errorf("Navigation: got %q, want %q", cfg.Navigation, "arrows")
	}
	if cfg.Theme != "auto" {
		t.Errorf("Theme: got %q, want %q", cfg.Theme, "auto")
	}

	// Dashboard defaults.
	if cfg.Dashboard.SortOrder != "newest" {
		t.Errorf("Dashboard.SortOrder: got %q, want %q", cfg.Dashboard.SortOrder, "newest")
	}
	if !cfg.Dashboard.ShowRecentlyDecided {
		t.Error("Dashboard.ShowRecentlyDecided: got false, want true")
	}
	if cfg.Dashboard.RecentlyDecidedLimit != 10 {
		t.Errorf("Dashboard.RecentlyDecidedLimit: got %d, want 10", cfg.Dashboard.RecentlyDecidedLimit)
	}

	// Detail defaults.
	if !cfg.Detail.ChatPanelOpen {
		t.Error("Detail.ChatPanelOpen: got false, want true")
	}
	if cfg.Detail.ChatPanelWidth != 35 {
		t.Errorf("Detail.ChatPanelWidth: got %d, want 35", cfg.Detail.ChatPanelWidth)
	}
	if cfg.Detail.ExpandCitationsDefault {
		t.Error("Detail.ExpandCitationsDefault: got true, want false")
	}

	// Personality defaults.
	if !cfg.Personality.FlavorText {
		t.Error("Personality.FlavorText: got false, want true")
	}
	if !cfg.Personality.EasterEggs {
		t.Error("Personality.EasterEggs: got false, want true")
	}

	// Confirmation defaults.
	if !cfg.Confirmations.ApproveRequiresConfirm {
		t.Error("Confirmations.ApproveRequiresConfirm: got false, want true")
	}
	if cfg.Confirmations.RejectRequiresConfirm {
		t.Error("Confirmations.RejectRequiresConfirm: got true, want false")
	}
	if cfg.Confirmations.DeferRequiresConfirm {
		t.Error("Confirmations.DeferRequiresConfirm: got true, want false")
	}
	if !cfg.Confirmations.RetryRequiresConfirm {
		t.Error("Confirmations.RetryRequiresConfirm: got false, want true")
	}
	if !cfg.Confirmations.RollbackRequiresConfirm {
		t.Error("Confirmations.RollbackRequiresConfirm: got false, want true")
	}

	// SourceManager defaults.
	if cfg.SourceManager.GroupCollapsedDefault {
		t.Error("SourceManager.GroupCollapsedDefault: got true, want false")
	}
}
