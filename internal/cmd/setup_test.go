package cmd

import (
	"strings"
	"testing"

	claude "github.com/vladolaru/cabrero/internal/integration/claude"
)

func TestHookGroupContainsCabrero(t *testing.T) {
	t.Run("nil returns false", func(t *testing.T) {
		if claude.HookGroupContainsCabrero(nil) {
			t.Error("expected false for nil")
		}
	})

	t.Run("non-slice returns false", func(t *testing.T) {
		if claude.HookGroupContainsCabrero("a string") {
			t.Error("expected false for string")
		}
	})

	t.Run("empty slice returns false", func(t *testing.T) {
		if claude.HookGroupContainsCabrero([]interface{}{}) {
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
		if claude.HookGroupContainsCabrero(v) {
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
		if !claude.HookGroupContainsCabrero(v) {
			t.Error("expected true with cabrero in command")
		}
	})
}

func TestRenderPlist(t *testing.T) {
	testPATH := daemonPATH()

	t.Run("renders valid plist with binary path", func(t *testing.T) {
		out, err := renderPlist("/usr/local/bin/cabrero", testPATH)
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
		out, err := renderPlist(path, testPATH)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, path) {
			t.Errorf("path %q not found in output", path)
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		out1, _ := renderPlist("/bin/cabrero", testPATH)
		out2, _ := renderPlist("/bin/cabrero", testPATH)
		if out1 != out2 {
			t.Error("renderPlist not deterministic")
		}
	})
}

func TestDaemonPATH(t *testing.T) {
	t.Run("base dirs always present", func(t *testing.T) {
		p := daemonPATH()
		for _, d := range []string{"/usr/local/bin", "/usr/bin", "/bin", "/opt/homebrew/bin"} {
			if !strings.Contains(p, d) {
				t.Errorf("missing base dir %s in PATH %q", d, p)
			}
		}
	})

	t.Run("extra dir appended", func(t *testing.T) {
		p := daemonPATH("/home/user/.local/bin")
		if !strings.HasSuffix(p, ":/home/user/.local/bin") {
			t.Errorf("extra dir not appended: %q", p)
		}
	})

	t.Run("duplicate dir not added", func(t *testing.T) {
		p := daemonPATH("/usr/local/bin")
		if strings.Count(p, "/usr/local/bin") != 1 {
			t.Errorf("duplicate dir in PATH: %q", p)
		}
	})

	t.Run("empty dir ignored", func(t *testing.T) {
		p := daemonPATH("")
		if strings.Contains(p, "::") {
			t.Errorf("empty dir produced double colon: %q", p)
		}
	})

	t.Run("PATH included in plist", func(t *testing.T) {
		extra := "/home/user/.local/bin"
		pathEnv := daemonPATH(extra)
		out, err := renderPlist("/bin/cabrero", pathEnv)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, extra) {
			t.Errorf("extra dir %q not found in plist output", extra)
		}
	})
}
