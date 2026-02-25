package shared

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-2 * 24 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		got := RelativeTime(c.t)
		if got != c.want {
			t.Errorf("RelativeTime(%v) = %q, want %q", time.Since(c.t).Round(time.Second), got, c.want)
		}
	}
}

func TestCheckmark(t *testing.T) {
	ok := Checkmark(true)
	if ok == "" {
		t.Error("Checkmark(true) should return non-empty string")
	}
	notOk := Checkmark(false)
	if notOk == "" {
		t.Error("Checkmark(false) should return non-empty string")
	}
	if ok == notOk {
		t.Error("Checkmark(true) and Checkmark(false) should differ")
	}
}
