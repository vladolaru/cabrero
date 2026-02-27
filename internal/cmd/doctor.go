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
	"time"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/daemon"
	claude "github.com/vladolaru/cabrero/internal/integration/claude"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

type checkStatus int

const (
	checkPass checkStatus = iota
	checkWarn
	checkFail
)

func (s checkStatus) String() string {
	switch s {
	case checkPass:
		return "pass"
	case checkWarn:
		return "warn"
	case checkFail:
		return "fail"
	default:
		return "unknown"
	}
}

func (s checkStatus) Icon() string {
	switch s {
	case checkPass:
		return cli.Success("✓")
	case checkWarn:
		return cli.Warn("!")
	case checkFail:
		return cli.Error("✗")
	default:
		return "?"
	}
}

type checkResult struct {
	name     string
	category string
	status   checkStatus
	message  string
	fixable  bool
	fixDesc  string
	fix      func() error
}

// jsonCheck is the JSON serialization format for a check result.
type jsonCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Fixable  bool   `json:"fixable,omitempty"`
	FixDesc  string `json:"fix_description,omitempty"`
}

type jsonOutput struct {
	Checks  []jsonCheck `json:"checks"`
	Summary jsonSummary `json:"summary"`
}

type jsonSummary struct {
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Fixed int `json:"fixed"`
}

// Doctor runs comprehensive diagnostics and offers to fix issues.
func Doctor(args []string, hooks EmbeddedHooks) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	autoFix := fs.Bool("fix", false, "auto-fix all fixable issues without prompting")
	jsonMode := fs.Bool("json", false, "output results as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	d := &doctorRunner{
		hooks:   hooks,
		autoFix: *autoFix,
		json:    *jsonMode,
	}

	return d.run()
}

type doctorRunner struct {
	hooks   EmbeddedHooks
	autoFix bool
	json    bool
}

func (d *doctorRunner) run() error {
	categories := []struct {
		name   string
		checks func() []checkResult
	}{
		{"Store", d.checkStore},
		{"Hook Scripts", d.checkHookScripts},
		{"Claude Code Integration", d.checkClaudeCodeIntegration},
		{"LaunchAgent", d.checkLaunchAgent},
		{"Daemon", d.checkDaemon},
		{"PATH", d.checkPath},
		{"Pipeline", d.checkPipeline},
	}

	var allResults []checkResult
	for _, cat := range categories {
		allResults = append(allResults, cat.checks()...)
	}

	if d.json {
		return d.outputJSON(allResults)
	}

	return d.outputHuman(allResults, categories)
}

func (d *doctorRunner) outputJSON(results []checkResult) error {
	var out jsonOutput
	for _, r := range results {
		jc := jsonCheck{
			Name:     r.name,
			Category: r.category,
			Status:   r.status.String(),
		}
		if r.message != "" {
			jc.Message = r.message
		}
		if r.fixable {
			jc.Fixable = true
			jc.FixDesc = r.fixDesc
		}
		out.Checks = append(out.Checks, jc)

		switch r.status {
		case checkPass:
			out.Summary.Pass++
		case checkWarn:
			out.Summary.Warn++
		case checkFail:
			out.Summary.Fail++
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func (d *doctorRunner) outputHuman(results []checkResult, categories []struct {
	name   string
	checks func() []checkResult
}) error {
	fmt.Println()
	fmt.Println(cli.Bold("Cabrero Doctor"))
	fmt.Println(cli.Accent(strings.Repeat("═", 40)))

	// Group results by category for display.
	resultsByCategory := make(map[string][]checkResult)
	for _, r := range results {
		resultsByCategory[r.category] = append(resultsByCategory[r.category], r)
	}

	for _, cat := range categories {
		fmt.Printf("\n%s\n", cli.Bold(cat.name))
		for _, r := range resultsByCategory[cat.name] {
			msg := r.name
			if r.message != "" {
				msg += " (" + r.message + ")"
			}
			fmt.Printf("  %s %s\n", r.status.Icon(), msg)
		}
	}

	// Summary.
	fails := 0
	warns := 0
	var fixable []checkResult
	var advisories []checkResult

	for _, r := range results {
		switch r.status {
		case checkFail:
			fails++
			if r.fixable {
				fixable = append(fixable, r)
			}
		case checkWarn:
			warns++
			advisories = append(advisories, r)
		}
	}

	fmt.Println()
	fmt.Println(cli.Accent(strings.Repeat("─", 40)))

	if fails == 0 && warns == 0 {
		fmt.Printf("Result: %s\n", cli.Success("all checks passed"))
		return nil
	}

	parts := []string{}
	if fails > 0 {
		parts = append(parts, cli.Error(fmt.Sprintf("%d issue(s)", fails)))
	}
	if warns > 0 {
		parts = append(parts, cli.Warn(fmt.Sprintf("%d warning(s)", warns)))
	}
	fmt.Printf("Result: %s\n", strings.Join(parts, ", "))

	// Fix flow.
	fixed := 0
	if len(fixable) > 0 {
		fmt.Printf("\n%s\n", cli.Bold("Fixable issues:"))
		for i, r := range fixable {
			fmt.Printf("  %d. %s\n", i+1, r.name)
			if r.message != "" {
				fmt.Printf("     %s\n", r.message)
			}
			fmt.Printf("     %s %s\n", cli.Accent("→"), r.fixDesc)

			if d.autoFix || d.confirmFix(r.name) {
				if err := r.fix(); err != nil {
					fmt.Printf("     %s Fix failed: %v\n", cli.Error("✗"), err)
				} else {
					fmt.Printf("     %s Fixed\n", cli.Success("✓"))
					fixed++
				}
			} else {
				fmt.Printf("     %s\n", cli.Muted("— Skipped"))
			}
			fmt.Println()
		}
	}

	// Non-fixable failures.
	var nonFixableFails []checkResult
	for _, r := range results {
		if r.status == checkFail && !r.fixable {
			nonFixableFails = append(nonFixableFails, r)
		}
	}

	if len(advisories) > 0 || len(nonFixableFails) > 0 {
		fmt.Printf("%s\n", cli.Bold("Advisories:"))
		for _, r := range nonFixableFails {
			msg := r.name
			if r.message != "" {
				msg += " — " + r.message
			}
			fmt.Printf("  %s %s\n", cli.Error("✗"), msg)
		}
		for _, r := range advisories {
			msg := r.name
			if r.message != "" {
				msg += " — " + r.message
			}
			fmt.Printf("  %s %s\n", cli.Warn("!"), msg)
		}
		fmt.Println()
	}

	if fixed > 0 && fixed == len(fixable) {
		fmt.Println(cli.Success("All fixable issues resolved."))
	}

	return nil
}

func (d *doctorRunner) confirmFix(name string) bool {
	fmt.Printf("     Fix? %s ", cli.Muted("[Y/n]"))
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}

// --- Check: Store ---

func (d *doctorRunner) checkStore() []checkResult {
	var results []checkResult
	root := store.Root()

	// Store directory exists and is writable.
	info, err := os.Stat(root)
	if err != nil {
		results = append(results, checkResult{
			name:     "Store directory exists",
			category: "Store",
			status:   checkFail,
			message:  fmt.Sprintf("%s not found", root),
			fixable:  true,
			fixDesc:  "Run store.Init() to create directory structure",
			fix:      func() error { return store.Init() },
		})
		return results
	}
	if !info.IsDir() {
		results = append(results, checkResult{
			name:     "Store directory exists",
			category: "Store",
			status:   checkFail,
			message:  fmt.Sprintf("%s exists but is not a directory", root),
		})
		return results
	}

	// Test writability.
	testFile := filepath.Join(root, ".doctor-write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		results = append(results, checkResult{
			name:     "Store directory exists",
			category: "Store",
			status:   checkFail,
			message:  fmt.Sprintf("%s is not writable", root),
		})
	} else {
		os.Remove(testFile)
		display := cli.ShortenHome(root)
		results = append(results, checkResult{
			name:     "Store directory exists",
			category: "Store",
			status:   checkPass,
			message:  display,
		})
	}

	// Check subdirectories.
	expectedDirs := []string{"raw", "digests", "prompts", "evaluations", "proposals"}
	missing := []string{}
	for _, sub := range expectedDirs {
		p := filepath.Join(root, sub)
		if _, err := os.Stat(p); err != nil {
			missing = append(missing, sub)
		}
	}
	if len(missing) > 0 {
		results = append(results, checkResult{
			name:     "All subdirectories present",
			category: "Store",
			status:   checkFail,
			message:  fmt.Sprintf("missing: %s", strings.Join(missing, ", ")),
			fixable:  true,
			fixDesc:  "Create missing subdirectories via store.Init()",
			fix:      func() error { return store.Init() },
		})
	} else {
		results = append(results, checkResult{
			name:     "All subdirectories present",
			category: "Store",
			status:   checkPass,
		})
	}

	// Blocklist.
	blPath := filepath.Join(root, "blocklist.json")
	_, err = os.ReadFile(blPath)
	if err != nil {
		results = append(results, checkResult{
			name:     "Blocklist file exists",
			category: "Store",
			status:   checkFail,
			message:  "blocklist.json not found",
			fixable:  true,
			fixDesc:  "Create blocklist.json via store.Init()",
			fix:      func() error { return store.Init() },
		})
	} else {
		if m, err := store.ReadBlocklist(); err != nil {
			results = append(results, checkResult{
				name:     "Blocklist valid",
				category: "Store",
				status:   checkFail,
				message:  fmt.Sprintf("invalid JSON: %v", err),
				fixable:  true,
				fixDesc:  "Recreate blocklist.json via store.Init()",
				fix:      func() error { return store.Init() },
			})
		} else {
			results = append(results, checkResult{
				name:     "Blocklist valid",
				category: "Store",
				status:   checkPass,
				message:  fmt.Sprintf("%d entries", len(m)),
			})
		}
	}

	return results
}

// --- Check: Hook Scripts ---

func (d *doctorRunner) checkHookScripts() []checkResult {
	var results []checkResult
	hooksDir := filepath.Join(store.Root(), "hooks")

	scripts := []struct {
		name     string
		embedded string
	}{
		{"pre-compact-backup.sh", d.hooks.PreCompact},
		{"session-end.sh", d.hooks.SessionEnd},
	}

	allExist := true
	allExecutable := true

	for _, script := range scripts {
		path := filepath.Join(hooksDir, script.name)
		info, err := os.Stat(path)
		if err != nil {
			allExist = false
			results = append(results, checkResult{
				name:     fmt.Sprintf("%s installed", script.name),
				category: "Hook Scripts",
				status:   checkFail,
				message:  "not found",
				fixable:  true,
				fixDesc:  fmt.Sprintf("Write %s from embedded version", path),
				fix:      d.makeInstallHookFix(path, script.embedded),
			})
			continue
		}

		// Check executable.
		if info.Mode()&0o111 == 0 {
			allExecutable = false
			results = append(results, checkResult{
				name:     fmt.Sprintf("%s executable", script.name),
				category: "Hook Scripts",
				status:   checkFail,
				message:  "not executable",
				fixable:  true,
				fixDesc:  fmt.Sprintf("chmod 0755 %s", path),
				fix: func() error {
					return os.Chmod(path, 0o755)
				},
			})
		}

		// Check content matches embedded.
		diskContent, err := os.ReadFile(path)
		if err == nil && string(diskContent) != script.embedded {
			results = append(results, checkResult{
				name:     fmt.Sprintf("%s current", script.name),
				category: "Hook Scripts",
				status:   checkFail,
				message:  "differs from current version",
				fixable:  true,
				fixDesc:  fmt.Sprintf("Overwrite %s with current version", path),
				fix:      d.makeInstallHookFix(path, script.embedded),
			})
		} else if err == nil {
			results = append(results, checkResult{
				name:     fmt.Sprintf("%s installed", script.name),
				category: "Hook Scripts",
				status:   checkPass,
			})
		}
	}

	if allExist && allExecutable {
		// Only add the aggregate "executable" check if all exist and we didn't
		// already report individual failures.
		hasExecFailure := false
		for _, r := range results {
			if strings.Contains(r.name, "executable") && r.status == checkFail {
				hasExecFailure = true
				break
			}
		}
		if !hasExecFailure {
			results = append(results, checkResult{
				name:     "Scripts are executable",
				category: "Hook Scripts",
				status:   checkPass,
			})
		}
	}

	return results
}

func (d *doctorRunner) makeInstallHookFix(path, content string) func() error {
	return func() error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(content), 0o755)
	}
}

// --- Check: Claude Code Integration ---

func (d *doctorRunner) checkClaudeCodeIntegration() []checkResult {
	var results []checkResult

	settingsPath, err := claude.SettingsPath()
	if err != nil {
		results = append(results, checkResult{
			name:     "Home directory accessible",
			category: "Claude Code Integration",
			status:   checkFail,
			message:  err.Error(),
		})
		return results
	}

	settings, _, err := claude.LoadSettings(settingsPath)
	if settings == nil && err == nil {
		results = append(results, checkResult{
			name:     "settings.json found",
			category: "Claude Code Integration",
			status:   checkFail,
			message:  "not found at " + settingsPath,
		})
		return results
	}
	if err != nil {
		results = append(results, checkResult{
			name:     "settings.json found",
			category: "Claude Code Integration",
			status:   checkFail,
			message:  fmt.Sprintf("failed to load: %v", err),
		})
		return results
	}

	results = append(results, checkResult{
		name:     "settings.json found",
		category: "Claude Code Integration",
		status:   checkPass,
	})

	hooksMap, _ := settings["hooks"].(map[string]interface{})
	hooksDir := filepath.Join(store.Root(), "hooks")
	preCompactPath := filepath.Join(hooksDir, "pre-compact-backup.sh")
	sessionEndPath := filepath.Join(hooksDir, "session-end.sh")

	// Check PreCompact hook.
	preCompactRegistered := claude.HookGroupContainsCabrero(hooksMap["PreCompact"])
	if preCompactRegistered {
		results = append(results, checkResult{
			name:     "PreCompact hook registered",
			category: "Claude Code Integration",
			status:   checkPass,
		})
		// Check that registered path actually exists.
		cmdPath := extractHookCommandPath(hooksMap["PreCompact"])
		if cmdPath != "" {
			if _, err := os.Stat(cmdPath); err != nil {
				results = append(results, checkResult{
					name:     "PreCompact hook path valid",
					category: "Claude Code Integration",
					status:   checkFail,
					message:  fmt.Sprintf("%s not found on disk", cmdPath),
					fixable:  true,
					fixDesc:  "Re-register hook with correct path",
					fix:      d.makeRegisterHooksFix(settings, settingsPath, hooksMap, preCompactPath, sessionEndPath),
				})
			}
		}
	} else {
		results = append(results, checkResult{
			name:     "PreCompact hook registered",
			category: "Claude Code Integration",
			status:   checkFail,
			message:  "cabrero hook not found in settings",
			fixable:  true,
			fixDesc:  "Register PreCompact hook in Claude Code settings",
			fix:      d.makeRegisterHooksFix(settings, settingsPath, hooksMap, preCompactPath, sessionEndPath),
		})
	}

	// Check SessionEnd hook.
	sessionEndRegistered := claude.HookGroupContainsCabrero(hooksMap["SessionEnd"])
	if sessionEndRegistered {
		results = append(results, checkResult{
			name:     "SessionEnd hook registered",
			category: "Claude Code Integration",
			status:   checkPass,
		})
		// Check that registered path actually exists.
		cmdPath := extractHookCommandPath(hooksMap["SessionEnd"])
		if cmdPath != "" {
			if _, err := os.Stat(cmdPath); err != nil {
				results = append(results, checkResult{
					name:     "SessionEnd hook path valid",
					category: "Claude Code Integration",
					status:   checkFail,
					message:  fmt.Sprintf("%s not found on disk", cmdPath),
					fixable:  true,
					fixDesc:  "Re-register hook with correct path",
					fix:      d.makeRegisterHooksFix(settings, settingsPath, hooksMap, preCompactPath, sessionEndPath),
				})
			}
		}
	} else {
		results = append(results, checkResult{
			name:     "SessionEnd hook registered",
			category: "Claude Code Integration",
			status:   checkFail,
			message:  "cabrero hook not found in settings",
			fixable:  true,
			fixDesc:  "Register SessionEnd hook in Claude Code settings",
			fix:      d.makeRegisterHooksFix(settings, settingsPath, hooksMap, preCompactPath, sessionEndPath),
		})
	}

	return results
}

// extractHookCommandPath finds the cabrero hook command path from a hook group.
func extractHookCommandPath(v interface{}) string {
	groups, ok := v.([]interface{})
	if !ok {
		return ""
	}
	for _, g := range groups {
		gm, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		hooks, ok := gm["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooks {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, _ := hm["command"].(string)
			if strings.Contains(cmd, "cabrero") {
				return cmd
			}
		}
	}
	return ""
}

func (d *doctorRunner) makeRegisterHooksFix(settings map[string]interface{}, settingsPath string, hooksMap map[string]interface{}, preCompactPath, sessionEndPath string) func() error {
	return func() error {
		if hooksMap == nil {
			hooksMap = make(map[string]interface{})
		}

		if !claude.HookGroupContainsCabrero(hooksMap["PreCompact"]) {
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

		if !claude.HookGroupContainsCabrero(hooksMap["SessionEnd"]) {
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

		return claude.WriteSettings(settings, settingsPath)
	}
}

// --- Check: LaunchAgent ---

func (d *doctorRunner) checkLaunchAgent() []checkResult {
	var results []checkResult

	if runtime.GOOS != "darwin" {
		results = append(results, checkResult{
			name:     "LaunchAgent check",
			category: "LaunchAgent",
			status:   checkPass,
			message:  "skipped (not macOS)",
		})
		return results
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return results
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")
	plistData, err := os.ReadFile(plistPath)
	if err != nil {
		results = append(results, checkResult{
			name:     "Plist installed",
			category: "LaunchAgent",
			status:   checkFail,
			message:  "com.cabrero.daemon.plist not found",
			fixable:  true,
			fixDesc:  "Generate and install LaunchAgent plist",
			fix:      d.makeInstallPlistFix(plistPath),
		})
		return results
	}

	results = append(results, checkResult{
		name:     "Plist installed",
		category: "LaunchAgent",
		status:   checkPass,
	})

	// Check binary path in plist points to existing file.
	binaryPath := extractPlistBinaryPath(string(plistData))
	if binaryPath != "" {
		if _, err := os.Stat(binaryPath); err != nil {
			results = append(results, checkResult{
				name:     "Plist binary path valid",
				category: "LaunchAgent",
				status:   checkFail,
				message:  fmt.Sprintf("points to %s (not found)", binaryPath),
				fixable:  true,
				fixDesc:  "Regenerate plist with correct binary path and reload",
				fix:      d.makeInstallPlistFix(plistPath),
			})
		} else {
			results = append(results, checkResult{
				name:     "Plist binary path valid",
				category: "LaunchAgent",
				status:   checkPass,
			})
		}
	}

	// Check plist content matches expected.
	expectedBinary := claude.DetermineBinaryPath()
	expectedContent, err := renderPlist(expectedBinary, daemonPATH(discoverClaudeDir()))
	if err == nil && string(plistData) != expectedContent {
		results = append(results, checkResult{
			name:     "Plist content current",
			category: "LaunchAgent",
			status:   checkFail,
			message:  "differs from expected content",
			fixable:  true,
			fixDesc:  "Regenerate plist with current configuration and reload",
			fix:      d.makeInstallPlistFix(plistPath),
		})
	} else if err == nil {
		results = append(results, checkResult{
			name:     "Plist content current",
			category: "LaunchAgent",
			status:   checkPass,
		})
	}

	return results
}

// extractPlistBinaryPath parses the first <string> in ProgramArguments from the plist XML.
func extractPlistBinaryPath(plist string) string {
	// Find ProgramArguments, then the first <string> after it.
	idx := strings.Index(plist, "<key>ProgramArguments</key>")
	if idx < 0 {
		return ""
	}
	rest := plist[idx:]
	// Find the <array> and the first <string> inside it.
	arrayIdx := strings.Index(rest, "<array>")
	if arrayIdx < 0 {
		return ""
	}
	rest = rest[arrayIdx:]
	startTag := "<string>"
	endTag := "</string>"
	start := strings.Index(rest, startTag)
	if start < 0 {
		return ""
	}
	start += len(startTag)
	end := strings.Index(rest[start:], endTag)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[start : start+end])
}

func (d *doctorRunner) makeInstallPlistFix(plistPath string) func() error {
	return func() error {
		binaryPath := claude.DetermineBinaryPath()
		content, err := renderPlist(binaryPath, daemonPATH(discoverClaudeDir()))
		if err != nil {
			return fmt.Errorf("generating plist: %w", err)
		}

		// Unload existing if present.
		if _, err := os.Stat(plistPath); err == nil {
			exec.Command("launchctl", "unload", plistPath).Run()
		}

		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return fmt.Errorf("creating LaunchAgents directory: %w", err)
		}

		if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing plist: %w", err)
		}

		if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
			return fmt.Errorf("loading plist: %w", err)
		}

		return nil
	}
}

// --- Check: Daemon ---

func (d *doctorRunner) checkDaemon() []checkResult {
	var results []checkResult

	pid, alive := daemon.IsDaemonRunning()
	if alive {
		results = append(results, checkResult{
			name:     "Daemon running",
			category: "Daemon",
			status:   checkPass,
			message:  fmt.Sprintf("PID %d", pid),
		})
	} else {
		// Build fix: try to load LaunchAgent on macOS.
		var fix func() error
		fixable := false
		fixDesc := "Start daemon manually with: cabrero daemon"

		if runtime.GOOS == "darwin" {
			home, err := os.UserHomeDir()
			if err == nil {
				plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")
				if _, err := os.Stat(plistPath); err == nil {
					fixable = true
					fixDesc = "Load LaunchAgent to start daemon"
					fix = func() error {
						return exec.Command("launchctl", "load", plistPath).Run()
					}
				}
			}
		}

		results = append(results, checkResult{
			name:     "Daemon running",
			category: "Daemon",
			status:   checkFail,
			message:  "not running",
			fixable:  fixable,
			fixDesc:  fixDesc,
			fix:      fix,
		})
	}

	// Check daemon log recency.
	logPath := filepath.Join(store.Root(), "daemon.log")
	logInfo, err := os.Stat(logPath)
	if err != nil {
		results = append(results, checkResult{
			name:     "Daemon log exists",
			category: "Daemon",
			status:   checkWarn,
			message:  "daemon.log not found",
		})
	} else {
		age := time.Since(logInfo.ModTime())
		if age > 24*time.Hour {
			results = append(results, checkResult{
				name:     "Daemon log recent",
				category: "Daemon",
				status:   checkWarn,
				message:  fmt.Sprintf("last entry %s — check %s", formatDuration(age), logPath),
			})
		} else {
			results = append(results, checkResult{
				name:     "Daemon log recent",
				category: "Daemon",
				status:   checkPass,
				message:  fmt.Sprintf("last entry %s", formatDuration(age)),
			})
		}
	}

	return results
}

// --- Check: PATH ---

func (d *doctorRunner) checkPath() []checkResult {
	var results []checkResult

	binDir := filepath.Join(store.Root(), "bin")

	// Check binary exists.
	binaryPath := filepath.Join(binDir, "cabrero")
	if _, err := os.Stat(binaryPath); err != nil {
		results = append(results, checkResult{
			name:     "Binary exists at ~/.cabrero/bin/cabrero",
			category: "PATH",
			status:   checkWarn,
			message:  "not found (may be installed elsewhere)",
		})
	} else {
		results = append(results, checkResult{
			name:     "Binary exists at ~/.cabrero/bin/cabrero",
			category: "PATH",
			status:   checkPass,
		})
	}

	// Check cabrero is reachable on PATH (covers symlinks like /usr/local/bin).
	display := cli.ShortenHome(binDir)

	if _, err := exec.LookPath("cabrero"); err == nil {
		results = append(results, checkResult{
			name:     fmt.Sprintf("%s reachable on PATH", display),
			category: "PATH",
			status:   checkPass,
		})
	} else {
		shell := filepath.Base(os.Getenv("SHELL"))
		var hint string
		switch shell {
		case "zsh":
			hint = fmt.Sprintf("Add to ~/.zshrc: export PATH=\"$HOME/.cabrero/bin:$PATH\"")
		case "bash":
			hint = fmt.Sprintf("Add to ~/.bashrc: export PATH=\"$HOME/.cabrero/bin:$PATH\"")
		default:
			hint = fmt.Sprintf("Add to shell profile: export PATH=\"$HOME/.cabrero/bin:$PATH\"")
		}
		results = append(results, checkResult{
			name:     fmt.Sprintf("%s reachable on PATH", display),
			category: "PATH",
			status:   checkFail,
			message:  hint,
		})
	}

	return results
}

// --- Check: Pipeline ---

func (d *doctorRunner) checkPipeline() []checkResult {
	var results []checkResult

	// Active models.
	cfg := pipeline.DefaultPipelineConfig()
	results = append(results, checkResult{
		name:     "Classifier model",
		category: "Pipeline",
		status:   checkPass,
		message:  cfg.ClassifierModel,
	})
	results = append(results, checkResult{
		name:     "Evaluator model",
		category: "Pipeline",
		status:   checkPass,
		message:  cfg.EvaluatorModel,
	})

	sessions, err := store.ListSessions()
	if err != nil {
		results = append(results, checkResult{
			name:     "Session store readable",
			category: "Pipeline",
			status:   checkFail,
			message:  err.Error(),
		})
		return results
	}

	// Sessions in error status.
	var errorSessions []store.Metadata
	for _, s := range sessions {
		if s.Status == "error" {
			errorSessions = append(errorSessions, s)
		}
	}

	if len(errorSessions) > 0 {
		msg := fmt.Sprintf("%d session(s) in error status", len(errorSessions))
		if len(errorSessions) <= 3 {
			ids := make([]string, len(errorSessions))
			for i, s := range errorSessions {
				id := s.SessionID
				if len(id) > 8 {
					id = id[:8]
				}
				ids[i] = id
			}
			msg += ": " + strings.Join(ids, ", ")
		}
		msg += " — retry with: cabrero run <session_id>"
		results = append(results, checkResult{
			name:     "Sessions in error status",
			category: "Pipeline",
			status:   checkWarn,
			message:  msg,
		})
	} else {
		results = append(results, checkResult{
			name:     "No sessions in error status",
			category: "Pipeline",
			status:   checkPass,
		})
	}

	// Sessions stuck in queued status for >24h.
	var stuckQueued []store.Metadata
	for _, s := range sessions {
		if s.Status != "queued" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, s.Timestamp)
		if err != nil {
			continue
		}
		if time.Since(ts) > 24*time.Hour {
			stuckQueued = append(stuckQueued, s)
		}
	}

	if len(stuckQueued) > 0 {
		results = append(results, checkResult{
			name:     "Stuck queued sessions",
			category: "Pipeline",
			status:   checkWarn,
			message:  fmt.Sprintf("%d session(s) queued >24h", len(stuckQueued)),
		})
	}

	// Prompt files exist.
	promptsDir := filepath.Join(store.Root(), "prompts")
	entries, err := os.ReadDir(promptsDir)
	if err != nil || len(entries) == 0 {
		results = append(results, checkResult{
			name:     "Prompt files present",
			category: "Pipeline",
			status:   checkWarn,
			message:  "no prompt files in " + promptsDir,
		})
	} else {
		results = append(results, checkResult{
			name:     "Prompt files present",
			category: "Pipeline",
			status:   checkPass,
			message:  fmt.Sprintf("%d file(s)", len(entries)),
		})
	}

	return results
}

// --- Helpers ---

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
