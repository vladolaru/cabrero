package shared

import "github.com/vladolaru/cabrero/internal/tui/message"

// HelpEntry is a single key binding with a full description.
type HelpEntry struct {
	Key  string // display text, e.g. "a" or "j/↓"
	Desc string // full description, e.g. "Approve this proposal and apply the change"
}

// HelpSection groups related help entries under a title.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// HelpForView returns help sections relevant to the given view.
func HelpForView(view message.ViewState, km KeyMap) []HelpSection {
	switch view {
	case message.ViewDashboard:
		return dashboardHelp(km)
	case message.ViewProposalDetail:
		return detailHelp(km)
	case message.ViewFitnessDetail:
		return fitnessHelp(km)
	case message.ViewSourceManager:
		return sourcesHelp(km)
	case message.ViewSourceDetail:
		return sourceDetailHelp(km)
	case message.ViewPipelineMonitor:
		return pipelineHelp(km)
	case message.ViewLogViewer:
		return logViewerHelp(km)
	default:
		return dashboardHelp(km)
	}
}

func dashboardHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{km.Up.Help().Key, "Move cursor up"},
				{km.Down.Help().Key, "Move cursor down"},
				{km.HalfPageUp.Help().Key, "Scroll up half a page"},
				{km.HalfPageDown.Help().Key, "Scroll down half a page"},
				{km.GotoTop.Help().Key, "Jump to first item"},
				{km.GotoBottom.Help().Key, "Jump to last item"},
			},
		},
		{
			Title: "Actions",
			Entries: []HelpEntry{
				{km.Open.Help().Key, "Open the selected proposal"},
				{km.Approve.Help().Key, "Approve and apply the selected proposal"},
				{km.Reject.Help().Key, "Reject the selected proposal"},
				{km.Defer.Help().Key, "Defer the selected proposal for later"},
				{km.Filter.Help().Key, "Filter proposals by text"},
				{km.Sort.Help().Key, "Cycle sort order"},
			},
		},
		{
			Title: "Views",
			Entries: []HelpEntry{
				{km.Sources.Help().Key, "Open source manager"},
				{km.Pipeline.Help().Key, "Open pipeline monitor"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}

func detailHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{km.Up.Help().Key, "Scroll up"},
				{km.Down.Help().Key, "Scroll down"},
				{km.HalfPageUp.Help().Key, "Scroll up half a page"},
				{km.HalfPageDown.Help().Key, "Scroll down half a page"},
				{km.GotoTop.Help().Key, "Jump to top"},
				{km.GotoBottom.Help().Key, "Jump to bottom"},
			},
		},
		{
			Title: "Actions",
			Entries: []HelpEntry{
				{km.Approve.Help().Key, "Approve and apply this proposal"},
				{km.Reject.Help().Key, "Reject this proposal"},
				{km.Defer.Help().Key, "Defer this proposal for later"},
			},
		},
		{
			Title: "Detail",
			Entries: []HelpEntry{
				{km.Chat.Help().Key, "Open AI chat about this proposal"},
				{km.UseRevision.Help().Key, "Use a specific revision"},
				{km.Chip1.Help().Key, "Quick prompt: why was this flagged?"},
				{km.Chip2.Help().Key, "Quick prompt: show the raw turns"},
				{km.Chip3.Help().Key, "Quick prompt: conservative version"},
				{km.Chip4.Help().Key, "Quick prompt: risk of approving?"},
			},
		},
		{
			Title: "Panes",
			Entries: []HelpEntry{
				{km.TabForward.Help().Key, "Switch to next pane"},
				{km.TabBackward.Help().Key, "Switch to previous pane"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to dashboard"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}

func fitnessHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{km.Up.Help().Key, "Scroll up"},
				{km.Down.Help().Key, "Scroll down"},
				{km.HalfPageUp.Help().Key, "Scroll up half a page"},
				{km.HalfPageDown.Help().Key, "Scroll down half a page"},
				{km.GotoTop.Help().Key, "Jump to top"},
				{km.GotoBottom.Help().Key, "Jump to bottom"},
			},
		},
		{
			Title: "Actions",
			Entries: []HelpEntry{
				{km.Open.Help().Key, "Open linked source"},
				{km.Dismiss.Help().Key, "Dismiss this report"},
			},
		},
		{
			Title: "Views",
			Entries: []HelpEntry{
				{km.Sources.Help().Key, "Open source manager"},
				{km.Chat.Help().Key, "Open AI chat about this report"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to dashboard"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}

func sourcesHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{km.Up.Help().Key, "Move cursor up"},
				{km.Down.Help().Key, "Move cursor down"},
				{km.Left.Help().Key, "Collapse group"},
				{km.Right.Help().Key, "Expand group"},
			},
		},
		{
			Title: "Actions",
			Entries: []HelpEntry{
				{km.Open.Help().Key, "Open source detail / toggle group"},
				{km.ToggleApproach.Help().Key, "Toggle approach (iterate/evaluate)"},
				{km.SetOwnership.Help().Key, "Set ownership (mine/not-mine)"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to previous view"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}

func sourceDetailHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Actions",
			Entries: []HelpEntry{
				{km.SetOwnership.Help().Key, "Set ownership (mine/not-mine)"},
				{km.ToggleApproach.Help().Key, "Toggle approach (iterate/evaluate)"},
				{km.Rollback.Help().Key, "Rollback latest change"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to source list"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}

func pipelineHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{km.Up.Help().Key, "Move cursor up"},
				{km.Down.Help().Key, "Move cursor down"},
			},
		},
		{
			Title: "Actions",
			Entries: []HelpEntry{
				{km.Open.Help().Key, "Open run detail"},
				{km.Retry.Help().Key, "Retry failed run"},
				{km.Refresh.Help().Key, "Refresh data"},
				{km.LogView.Help().Key, "View daemon logs"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to dashboard"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}

func logViewerHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Search",
			Entries: []HelpEntry{
				{km.Search.Help().Key, "Start search"},
				{km.SearchNext.Help().Key, "Jump to next match"},
				{km.SearchPrev.Help().Key, "Jump to previous match"},
			},
		},
		{
			Title: "View",
			Entries: []HelpEntry{
				{km.FollowToggle.Help().Key, "Toggle follow mode (auto-scroll)"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to pipeline monitor"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}
