package cmd

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/vladolaru/cabrero/internal/daemon"
	"github.com/vladolaru/cabrero/internal/store"
)

// EmbeddedHooks holds hook script content embedded in the binary via //go:embed.
type EmbeddedHooks struct {
	PreCompact string
	SessionEnd string
}

// Setup runs the interactive setup wizard.
func Setup(args []string, hooks EmbeddedHooks) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	autoYes := fs.Bool("yes", false, "skip all confirmations")
	dryRun := fs.Bool("dry-run", false, "show what would be done without doing it")
	hooksOnly := fs.Bool("hooks-only", false, "only update hook scripts (used by cabrero update)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s := &setupRunner{
		hooks:   hooks,
		autoYes: *autoYes,
		dryRun:  *dryRun,
	}

	if *hooksOnly {
		return s.stepInstallHookScripts(3, 3)
	}

	return s.run()
}

type setupRunner struct {
	hooks   EmbeddedHooks
	autoYes bool
	dryRun  bool
}

func (s *setupRunner) run() error {
	fmt.Println()
	fmt.Println("Cabrero Setup")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Println()

	steps := []struct {
		name string
		fn   func(step, total int) error
	}{
		{"Check prerequisites", s.stepPrerequisites},
		{"Initialize store", s.stepInitStore},
		{"Install hook scripts", s.stepInstallHookScripts},
		{"Register hooks in Claude Code", s.stepRegisterHooks},
		{"Install LaunchAgent", s.stepInstallLaunchAgent},
		{"Start daemon", s.stepStartDaemon},
		{"PATH check", s.stepPathCheck},
		{"Process existing sessions", s.stepBackfillOffer},
	}

	total := len(steps)
	for i, step := range steps {
		fmt.Printf("Step %d/%d: %s\n", i+1, total, step.name)
		if err := step.fn(i+1, total); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, step.name, err)
		}
		fmt.Println()
	}

	fmt.Println("Setup complete.")
	return nil
}

// Step 1: Check prerequisites.
func (s *setupRunner) stepPrerequisites(step, total int) error {
	// Check claude CLI.
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		fmt.Println("  ! claude CLI not found in PATH")
		fmt.Println("    Capture will work but pipeline execution requires the claude CLI.")
		fmt.Println("    Install it from: https://docs.anthropic.com/en/docs/claude-code")
	} else {
		ver := "unknown"
		out, err := exec.Command(claudePath, "--version").Output()
		if err == nil {
			ver = strings.TrimSpace(string(out))
		}
		fmt.Printf("  ✓ claude CLI found (%s)\n", ver)
	}

	// Check macOS.
	if runtime.GOOS == "darwin" {
		fmt.Println("  ✓ macOS detected")
	} else {
		fmt.Printf("  ! %s detected — LaunchAgent setup will be skipped\n", runtime.GOOS)
	}

	return nil
}

// Step 2: Initialize store.
func (s *setupRunner) stepInitStore(step, total int) error {
	if s.dryRun {
		fmt.Printf("  → Would initialize store at %s\n", store.Root())
		return nil
	}
	if err := store.Init(); err != nil {
		return err
	}
	fmt.Printf("  ✓ Store initialized at %s\n", store.Root())
	return nil
}

// Step 3: Install hook scripts from embedded content.
func (s *setupRunner) stepInstallHookScripts(step, total int) error {
	hooksDir := filepath.Join(store.Root(), "hooks")

	if s.dryRun {
		fmt.Printf("  → Would write %s/pre-compact-backup.sh\n", hooksDir)
		fmt.Printf("  → Would write %s/session-end.sh\n", hooksDir)
		return nil
	}

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	scripts := []struct {
		name    string
		content string
	}{
		{"pre-compact-backup.sh", s.hooks.PreCompact},
		{"session-end.sh", s.hooks.SessionEnd},
	}

	for _, script := range scripts {
		path := filepath.Join(hooksDir, script.name)
		existing, err := os.ReadFile(path)
		if err == nil && string(existing) == script.content {
			fmt.Printf("  ✓ %s already installed\n", script.name)
			continue
		}

		fmt.Printf("  → Writing %s\n", path)
		if err := os.WriteFile(path, []byte(script.content), 0o755); err != nil {
			return fmt.Errorf("writing %s: %w", script.name, err)
		}
	}

	fmt.Println("  ✓ Hook scripts installed")
	return nil
}

// Step 4: Register hooks in Claude Code settings.json.
func (s *setupRunner) stepRegisterHooks(step, total int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	hooksDir := filepath.Join(store.Root(), "hooks")

	// Read existing settings.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ! Claude Code settings not found — skipping hook registration")
			fmt.Printf("    Expected: %s\n", settingsPath)
			return nil
		}
		return fmt.Errorf("reading settings: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	// Check existing hooks.
	preCompactPath := filepath.Join(hooksDir, "pre-compact-backup.sh")
	sessionEndPath := filepath.Join(hooksDir, "session-end.sh")

	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = make(map[string]interface{})
	}

	preCompactRegistered := hookGroupContainsCabrero(hooksMap["PreCompact"])
	sessionEndRegistered := hookGroupContainsCabrero(hooksMap["SessionEnd"])

	if preCompactRegistered && sessionEndRegistered {
		fmt.Println("  ✓ Hooks already registered")
		return nil
	}

	// Show what will be done.
	fmt.Printf("  Claude Code settings: %s\n", settingsPath)
	if !preCompactRegistered {
		fmt.Printf("  Will add: PreCompact hook → %s\n", preCompactPath)
	}
	if !sessionEndRegistered {
		fmt.Printf("  Will add: SessionEnd hook → %s\n", sessionEndPath)
	}

	if s.dryRun {
		fmt.Println("  → Would register hooks (dry-run)")
		return nil
	}

	if !s.confirm("Register hooks?") {
		fmt.Println("  — Skipped")
		return nil
	}

	// Add missing hooks.
	if !preCompactRegistered {
		existing, _ := hooksMap["PreCompact"].([]interface{})
		hooksMap["PreCompact"] = append(existing, map[string]interface{}{
			"matcher": "",
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": preCompactPath,
					"timeout": 30,
				},
			},
		})
	}

	if !sessionEndRegistered {
		existing, _ := hooksMap["SessionEnd"].([]interface{})
		hooksMap["SessionEnd"] = append(existing, map[string]interface{}{
			"matcher": "",
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": sessionEndPath,
					"timeout": 30,
				},
			},
		})
	}

	settings["hooks"] = hooksMap

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	fmt.Println("  ✓ Hooks registered")
	return nil
}

// hookGroupContainsCabrero checks if a hook group ([]interface{}) contains a cabrero hook.
func hookGroupContainsCabrero(v interface{}) bool {
	groups, ok := v.([]interface{})
	if !ok {
		return false
	}
	raw, _ := json.Marshal(groups)
	return strings.Contains(string(raw), "cabrero")
}

// Step 5: Install LaunchAgent.
func (s *setupRunner) stepInstallLaunchAgent(step, total int) error {
	if runtime.GOOS != "darwin" {
		fmt.Println("  — Skipped (not macOS)")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")

	// Determine binary path: prefer ~/.cabrero/bin/cabrero, fall back to current executable.
	binaryPath := filepath.Join(store.Root(), "bin", "cabrero")
	if _, err := os.Stat(binaryPath); err != nil {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine binary path: %w", err)
		}
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			binaryPath = resolved
		} else {
			binaryPath = exe
		}
	}

	plistContent, err := renderPlist(binaryPath)
	if err != nil {
		return fmt.Errorf("generating plist: %w", err)
	}

	// Check if already installed and matches.
	existing, readErr := os.ReadFile(plistPath)
	if readErr == nil && string(existing) == plistContent {
		fmt.Println("  ✓ LaunchAgent already installed")
		return nil
	}

	fmt.Printf("  Will install: %s\n", plistPath)
	fmt.Printf("  Binary path:  %s\n", binaryPath)

	if s.dryRun {
		fmt.Println("  → Would install LaunchAgent (dry-run)")
		return nil
	}

	if !s.confirm("Install LaunchAgent for background processing?") {
		fmt.Println("  — Skipped")
		return nil
	}

	// Unload existing if present.
	if readErr == nil {
		exec.Command("launchctl", "unload", plistPath).Run() // ignore error
	}

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("creating LaunchAgents directory: %w", err)
	}

	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("loading LaunchAgent: %w", err)
	}

	fmt.Println("  ✓ LaunchAgent installed and loaded")
	return nil
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.cabrero.daemon</string>

	<key>ProgramArguments</key>
	<array>
		<string>{{.BinaryPath}}</string>
		<string>daemon</string>
	</array>

	<key>RunAtLoad</key>
	<true/>

	<key>KeepAlive</key>
	<true/>

	<key>ProcessType</key>
	<string>Background</string>

	<key>LowPriorityIO</key>
	<true/>

	<key>Nice</key>
	<integer>10</integer>

	<key>StandardOutPath</key>
	<string>/tmp/cabrero-daemon-stdout.log</string>

	<key>StandardErrorPath</key>
	<string>/tmp/cabrero-daemon-stderr.log</string>

	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
	</dict>
</dict>
</plist>
`

func renderPlist(binaryPath string) (string, error) {
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, struct{ BinaryPath string }{binaryPath}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Step 6: Start daemon.
func (s *setupRunner) stepStartDaemon(step, total int) error {
	if s.dryRun {
		fmt.Println("  → Would check/start daemon (dry-run)")
		return nil
	}

	if pid, alive := daemon.IsDaemonRunning(); alive {
		fmt.Printf("  ✓ Daemon running (PID %d)\n", pid)
		return nil
	}

	// If LaunchAgent is installed, it should have started via RunAtLoad.
	// Give it a moment to come up.
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")
		if _, err := os.Stat(plistPath); err == nil {
			// Try loading again in case it wasn't loaded.
			exec.Command("launchctl", "load", plistPath).Run()

			// Brief wait and recheck.
			if pid, alive := daemon.IsDaemonRunning(); alive {
				fmt.Printf("  ✓ Daemon running (PID %d)\n", pid)
				return nil
			}
		}
	}

	fmt.Println("  ! Daemon not running")
	fmt.Println("    Start manually with: cabrero daemon")
	return nil
}

// Step 7: PATH check.
func (s *setupRunner) stepPathCheck(step, total int) error {
	// Check if "cabrero" resolves anywhere on PATH (covers symlinks in
	// /usr/local/bin, ~/.cabrero/bin in PATH, etc.).
	if path, err := exec.LookPath("cabrero"); err == nil {
		fmt.Printf("  ✓ cabrero is on PATH (%s)\n", path)
		return nil
	}

	fmt.Println("  ! cabrero is not on PATH")
	fmt.Println()
	fmt.Println("    Either symlink into /usr/local/bin:")
	fmt.Println("      sudo ln -sf ~/.cabrero/bin/cabrero /usr/local/bin/cabrero")
	fmt.Println()
	fmt.Println("    Or add ~/.cabrero/bin to your PATH:")

	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "zsh":
		fmt.Println("      echo 'export PATH=\"$HOME/.cabrero/bin:$PATH\"' >> ~/.zshrc")
		fmt.Println("      source ~/.zshrc")
	case "bash":
		fmt.Println("      echo 'export PATH=\"$HOME/.cabrero/bin:$PATH\"' >> ~/.bashrc")
		fmt.Println("      source ~/.bashrc")
	default:
		fmt.Println("      export PATH=\"$HOME/.cabrero/bin:$PATH\"")
	}

	return nil
}

// Step 8: Offer to import and process existing sessions.
func (s *setupRunner) stepBackfillOffer(step, total int) error {
	if s.dryRun {
		fmt.Println("  → Would offer to import and process existing sessions (dry-run)")
		return nil
	}

	// Import existing CC sessions (quiet mode — summary only).
	fmt.Println("  Scanning for existing CC sessions...")
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("  Warning: cannot determine home directory: %v\n", err)
		return nil
	}
	from := filepath.Join(home, ".claude", "projects")
	result, importErr := RunImport(from, false, true)
	if importErr != nil {
		fmt.Printf("  Warning: import scan failed: %v\n", importErr)
	} else if result.Imported > 0 {
		fmt.Printf("  Imported %d session(s) (%d already present)\n", result.Imported, result.Skipped)
	}

	// Count pending sessions.
	sessions, err := store.QuerySessions(store.SessionFilter{
		Statuses: []string{"imported"},
	})
	if err != nil || len(sessions) == 0 {
		fmt.Println("  No existing sessions to process.")
		return nil
	}

	fmt.Printf("  Found %d session(s) ready for processing\n", len(sessions))

	if !s.confirm("Enqueue recent sessions for background processing?") {
		fmt.Println("  — Skipped. Run 'cabrero backfill --enqueue' later to process.")
		return nil
	}

	// Ask how far back.
	sinceDate := time.Now().AddDate(0, -1, 0)
	if !s.autoYes {
		fmt.Printf("  How far back? (default: 1 month, or enter YYYY-MM-DD) ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			t, err := time.Parse("2006-01-02", input)
			if err != nil {
				fmt.Printf("  Could not parse date %q, using default (1 month)\n", input)
			} else {
				sinceDate = t
			}
		}
	}

	fmt.Printf("  Processing sessions since %s...\n\n", sinceDate.Format("2006-01-02"))

	return Backfill([]string{
		"--since", sinceDate.Format("2006-01-02"),
		"--enqueue",
		"--yes",
	})
}

// confirm prompts the user for Y/n. Returns true on Y or empty input.
func (s *setupRunner) confirm(prompt string) bool {
	if s.autoYes {
		fmt.Printf("  %s [Y/n] y (--yes)\n", prompt)
		return true
	}

	fmt.Printf("  %s [Y/n] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}
