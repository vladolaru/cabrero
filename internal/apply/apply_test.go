package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTarget_TraversalEscapingHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	// Craft a path that resolves outside home after filepath.Clean.
	escaped := filepath.Clean(filepath.Join(home, "../../etc/hosts.md"))
	if strings.HasPrefix(escaped, home+string(filepath.Separator)) {
		t.Skipf("path %s still inside home — unusual test environment", escaped)
	}
	if err := validateTarget(escaped); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for path outside home", escaped)
	}
}

func TestValidateTarget_ValidInsideHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	valid := filepath.Join(home, ".claude", "SKILL.md")
	if err := validateTarget(valid); err != nil {
		t.Errorf("validateTarget(%q) = %v, want nil", valid, err)
	}
}

func TestValidateTarget_NotMarkdown(t *testing.T) {
	home, _ := os.UserHomeDir()
	notMd := filepath.Join(home, ".claude", "script.sh")
	if err := validateTarget(notMd); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for non-.md", notMd)
	}
}

func TestValidateTarget_AtHomeRoot(t *testing.T) {
	// A path exactly equal to home (without a child) must be rejected.
	home, _ := os.UserHomeDir()
	if err := validateTarget(home); err == nil {
		t.Errorf("validateTarget(%q) = nil, want error for home root", home)
	}
}
