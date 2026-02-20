// Package shared contains types shared between the tui root and its child packages.
// This exists to break import cycles: the root tui package and child packages
// (dashboard, detail, chat, testdata) both import shared instead of each other.
package shared

import "encoding/json"

// Config holds all TUI configuration. Stored at ~/.cabrero/config.json.
// Missing fields get defaults. Unknown fields are preserved on roundtrip.
type Config struct {
	Navigation    string              `json:"navigation"`
	Theme         string              `json:"theme"`
	Dashboard     DashboardConfig     `json:"dashboard"`
	Detail        DetailConfig        `json:"detail"`
	Personality   PersonalityConfig   `json:"personality"`
	Confirmations ConfirmConfig       `json:"confirmations"`
	SourceManager SourceManagerConfig `json:"sourceManager"`
	Pipeline      PipelineConfig      `json:"pipeline"`

	// Extra preserves unknown JSON fields for forward compatibility.
	Extra map[string]json.RawMessage `json:"-"`
}

// DashboardConfig holds dashboard-specific settings.
type DashboardConfig struct {
	SortOrder            string `json:"sortOrder"`
	ShowRecentlyDecided  bool   `json:"showRecentlyDecided"`
	RecentlyDecidedLimit int    `json:"recentlyDecidedLimit"`
}

// DetailConfig holds proposal detail view settings.
type DetailConfig struct {
	ChatPanelOpen          bool `json:"chatPanelOpen"`
	ChatPanelWidth         int  `json:"chatPanelWidth"`
	ExpandCitationsDefault bool `json:"expandCitationsDefault"`
}

// PersonalityConfig controls pirategoat personality features.
type PersonalityConfig struct {
	FlavorText bool `json:"flavorText"`
	EasterEggs bool `json:"easterEggs"`
}

// SourceManagerConfig holds source manager view settings.
type SourceManagerConfig struct {
	GroupCollapsedDefault bool `json:"groupCollapsedDefault"`
}

// PipelineConfig holds pipeline monitor view settings.
type PipelineConfig struct {
	SparklineDays   int  `json:"sparklineDays"`
	RecentRunsLimit int  `json:"recentRunsLimit"`
	LogFollowMode   bool `json:"logFollowMode"`
}

// ConfirmConfig controls which actions require confirmation.
type ConfirmConfig struct {
	ApproveRequiresConfirm  bool `json:"approveRequiresConfirm"`
	RejectRequiresConfirm   bool `json:"rejectRequiresConfirm"`
	DeferRequiresConfirm    bool `json:"deferRequiresConfirm"`
	RetryRequiresConfirm    bool `json:"retryRequiresConfirm"`
	RollbackRequiresConfirm bool `json:"rollbackRequiresConfirm"`
}

// DefaultConfig returns a Config with all design-doc default values.
func DefaultConfig() *Config {
	return &Config{
		Navigation: "arrows",
		Theme:      "auto",
		Dashboard: DashboardConfig{
			SortOrder:            "newest",
			ShowRecentlyDecided:  true,
			RecentlyDecidedLimit: 10,
		},
		Detail: DetailConfig{
			ChatPanelOpen:          true,
			ChatPanelWidth:         35,
			ExpandCitationsDefault: false,
		},
		Personality: PersonalityConfig{
			FlavorText: true,
			EasterEggs: true,
		},
		Confirmations: ConfirmConfig{
			ApproveRequiresConfirm:  true,
			RejectRequiresConfirm:   false,
			DeferRequiresConfirm:    false,
			RetryRequiresConfirm:    true,
			RollbackRequiresConfirm: true,
		},
		SourceManager: SourceManagerConfig{
			GroupCollapsedDefault: false,
		},
		Pipeline: PipelineConfig{
			SparklineDays:   7,
			RecentRunsLimit: 20,
			LogFollowMode:   true,
		},
	}
}
