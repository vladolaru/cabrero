package pipeline

import "testing"

func TestShortID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"longer than 8 truncates", "abcdef1234567890", "abcdef12"},
		{"exactly 8 unchanged", "abcdef12", "abcdef12"},
		{"shorter than 8 unchanged", "abc", "abc"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shortID(tc.input)
			if got != tc.want {
				t.Errorf("shortID(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFilterProposals(t *testing.T) {
	t.Run("returns shallow copy not same pointer", func(t *testing.T) {
		out := &EvaluatorOutput{SessionID: "sess-abc", Proposals: []Proposal{}}
		got := filterProposals(out, "prop-")
		if got == out {
			t.Error("filterProposals returned same pointer, want a copy")
		}
		if got.SessionID != out.SessionID {
			t.Errorf("SessionID = %q, want %q", got.SessionID, out.SessionID)
		}
	})

	t.Run("nil proposals returns empty slice", func(t *testing.T) {
		out := &EvaluatorOutput{Proposals: nil}
		got := filterProposals(out, "prop-abcd1234-")
		if got.Proposals == nil {
			t.Error("Proposals is nil, want empty slice")
		}
		if len(got.Proposals) != 0 {
			t.Errorf("len(Proposals) = %d, want 0", len(got.Proposals))
		}
	})

	t.Run("keeps only matching prefix", func(t *testing.T) {
		out := &EvaluatorOutput{
			Proposals: []Proposal{
				{ID: "prop-abcd1234-0"},
				{ID: "prop-abcd1234-1"},
				{ID: "prop-efgh5678-0"},
			},
		}
		got := filterProposals(out, "prop-abcd1234-")
		if len(got.Proposals) != 2 {
			t.Fatalf("got %d proposals, want 2", len(got.Proposals))
		}
		for _, p := range got.Proposals {
			if p.ID != "prop-abcd1234-0" && p.ID != "prop-abcd1234-1" {
				t.Errorf("unexpected proposal ID %q", p.ID)
			}
		}
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		out := &EvaluatorOutput{
			Proposals: []Proposal{{ID: "prop-efgh5678-0"}},
		}
		got := filterProposals(out, "prop-abcd1234-")
		if len(got.Proposals) != 0 {
			t.Errorf("got %d proposals, want 0", len(got.Proposals))
		}
	})

	t.Run("does not modify original", func(t *testing.T) {
		original := &EvaluatorOutput{
			Proposals: []Proposal{
				{ID: "prop-abcd1234-0"},
				{ID: "prop-efgh5678-0"},
			},
		}
		filterProposals(original, "prop-abcd1234-")
		if len(original.Proposals) != 2 {
			t.Errorf("original modified: got %d proposals, want 2", len(original.Proposals))
		}
	})

	t.Run("empty prefix matches all", func(t *testing.T) {
		out := &EvaluatorOutput{
			Proposals: []Proposal{
				{ID: "prop-abcd1234-0"},
				{ID: "prop-efgh5678-0"},
			},
		}
		got := filterProposals(out, "")
		if len(got.Proposals) != 2 {
			t.Errorf("got %d proposals, want 2", len(got.Proposals))
		}
	})
}
