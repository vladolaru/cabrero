package cmd

import (
	"strings"
	"testing"
)

func TestHookGroupContainsCabrero(t *testing.T) {
	t.Run("nil returns false", func(t *testing.T) {
		if hookGroupContainsCabrero(nil) {
			t.Error("expected false for nil")
		}
	})

	t.Run("non-slice returns false", func(t *testing.T) {
		if hookGroupContainsCabrero("a string") {
			t.Error("expected false for string")
		}
	})

	t.Run("empty slice returns false", func(t *testing.T) {
		if hookGroupContainsCabrero([]interface{}{}) {
			t.Error("expected false for empty slice")
		}
	})

	t.Run("slice without cabrero returns false", func(t *testing.T) {
		v := []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{"command": "/usr/local/bin/other-tool"},
				},
			},
		}
		if hookGroupContainsCabrero(v) {
			t.Error("expected false without cabrero")
		}
	})

	t.Run("slice with cabrero in command returns true", func(t *testing.T) {
		v := []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{"command": "/home/user/.cabrero/hooks/session-end.sh"},
				},
			},
		}
		if !hookGroupContainsCabrero(v) {
			t.Error("expected true with cabrero in command")
		}
	})
}

func TestRenderPlist(t *testing.T) {
	t.Run("renders valid plist with binary path", func(t *testing.T) {
		out, err := renderPlist("/usr/local/bin/cabrero")
		if err != nil {
			t.Fatalf("renderPlist: %v", err)
		}
		if !strings.HasPrefix(out, "<?xml") {
			t.Error("output missing XML declaration")
		}
		if !strings.Contains(out, "/usr/local/bin/cabrero") {
			t.Error("output missing binary path")
		}
		if !strings.Contains(out, "com.cabrero.daemon") {
			t.Error("output missing daemon label")
		}
		if !strings.Contains(out, "</plist>") {
			t.Error("output missing closing plist tag")
		}
	})

	t.Run("path with spaces included verbatim", func(t *testing.T) {
		path := "/Users/my user/.cabrero/bin/cabrero"
		out, err := renderPlist(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, path) {
			t.Errorf("path %q not found in output", path)
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		out1, _ := renderPlist("/bin/cabrero")
		out2, _ := renderPlist("/bin/cabrero")
		if out1 != out2 {
			t.Error("renderPlist not deterministic")
		}
	})
}
