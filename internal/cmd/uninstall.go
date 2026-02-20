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
	"strconv"
	"strings"
	"syscall"

	"github.com/vladolaru/cabrero/internal/daemon"
	"github.com/vladolaru/cabrero/internal/store"
)

// Uninstall removes Cabrero from the system, reversing what setup did.
func Uninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	autoYes := fs.Bool("yes", false, "skip all confirmations")
	keepData := fs.Bool("keep-data", false, "keep ~/.cabrero data directory")
	removeData := fs.Bool("remove-data", false, "remove ~/.cabrero data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *keepData && *removeData {
		return fmt.Errorf("--keep-data and --remove-data are mutually exclusive")
	}

	u := &uninstallRunner{
		autoYes:    *autoYes,
		keepData:   *keepData,
		removeData: *removeData,
	}

	return u.run()
}

type uninstallRunner struct {
	autoYes    bool
	keepData   bool
	removeData bool
}

func (u *uninstallRunner) run() error {
	fmt.Println()
	fmt.Println("Cabrero Uninstall")
	fmt.Println(strings.Repeat("═", 40))
	fmt.Println()

	steps := []struct {
		name string
		fn   func(step, total int) error
	}{
		{"Stop daemon", u.stepStopDaemon},
		{"Remove LaunchAgent", u.stepRemoveLaunchAgent},
		{"Remove daemon logs", u.stepRemoveDaemonLogs},
		{"Unregister Claude Code hooks", u.stepUnregisterHooks},
		{"Remove hook scripts", u.stepRemoveHookScripts},
		{"Remove binary", u.stepRemoveBinary},
		{"Data directory", u.stepDataDirectory},
	}

	total := len(steps)
	for i, step := range steps {
		fmt.Printf("Step %d/%d: %s\n", i+1, total, step.name)
		if err := step.fn(i+1, total); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, step.name, err)
		}
		fmt.Println()
	}

	// PATH reminder.
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(store.Root(), "bin")
	display := strings.Replace(binDir, home, "~", 1)

	shell := filepath.Base(os.Getenv("SHELL"))
	var profile string
	switch shell {
	case "zsh":
		profile = "~/.zshrc"
	case "bash":
		profile = "~/.bashrc or ~/.bash_profile"
	default:
		profile = "your shell profile"
	}

	fmt.Printf("If you added %s to your PATH, remove the export line from\n%s.\n\n", display, profile)
	fmt.Println("Cabrero uninstalled.")
	return nil
}

// Step 1: Stop daemon.
func (u *uninstallRunner) stepStopDaemon(step, total int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Try launchctl unload first on macOS.
	if runtime.GOOS == "darwin" {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")
		if _, err := os.Stat(plistPath); err == nil {
			exec.Command("launchctl", "unload", plistPath).Run() // ignore error
		}
	}

	// Check if daemon is still running and kill it.
	pid, alive := daemon.IsDaemonRunning()
	if !alive {
		fmt.Println("  ✓ Daemon not running (already stopped)")
		return nil
	}

	// Send SIGTERM.
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		fmt.Printf("  ✓ Daemon not running (PID %d unreachable)\n", pid)
		return nil
	}

	// Verify it stopped.
	if _, still := daemon.IsDaemonRunning(); still {
		fmt.Printf("  ! Daemon (PID %d) did not stop after SIGTERM\n", pid)
		fmt.Println("    You may need to kill it manually: kill", pid)
	} else {
		fmt.Printf("  ✓ Daemon stopped (was PID %d)\n", pid)
	}

	// Clean up PID file.
	pidPath := filepath.Join(store.Root(), "daemon.pid")
	os.Remove(pidPath) // ignore error

	return nil
}

// Step 2: Remove LaunchAgent plist.
func (u *uninstallRunner) stepRemoveLaunchAgent(step, total int) error {
	if runtime.GOOS != "darwin" {
		fmt.Println("  — Skipped (not macOS)")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("  ✓ LaunchAgent not present (already removed)")
		return nil
	}

	if err := os.Remove(plistPath); err != nil {
		return fmt.Errorf("removing plist: %w", err)
	}

	display := strings.Replace(plistPath, home, "~", 1)
	fmt.Printf("  ✓ Removed %s\n", display)
	return nil
}

// Step 3: Remove /tmp daemon logs.
func (u *uninstallRunner) stepRemoveDaemonLogs(step, total int) error {
	logs := []string{
		"/tmp/cabrero-daemon-stdout.log",
		"/tmp/cabrero-daemon-stderr.log",
	}

	for _, path := range logs {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		if err := os.Remove(path); err != nil {
			fmt.Printf("  ! Could not remove %s: %v\n", path, err)
			continue
		}
		fmt.Printf("  ✓ Removed %s\n", path)
	}

	return nil
}

// Step 4: Unregister Claude Code hooks.
func (u *uninstallRunner) stepUnregisterHooks(step, total int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  ✓ No settings.json found (nothing to unregister)")
			return nil
		}
		return fmt.Errorf("reading settings: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	hooksMap, _ := settings["hooks"].(map[string]interface{})
	if hooksMap == nil {
		fmt.Println("  ✓ No hooks registered (nothing to unregister)")
		return nil
	}

	removed := filterCabreroHooks(hooksMap)
	if len(removed) == 0 {
		fmt.Println("  ✓ No cabrero hooks found (nothing to unregister)")
		return nil
	}

	// Write back settings without cabrero hooks.
	settings["hooks"] = hooksMap
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	for _, name := range removed {
		fmt.Printf("  ✓ Removed %s hook from settings.json\n", name)
	}
	return nil
}

// filterCabreroHooks removes hook group entries containing "cabrero" from the
// hooks map. It modifies the map in place and returns the names of hook groups
// that had entries removed.
func filterCabreroHooks(hooksMap map[string]interface{}) []string {
	var removed []string

	for groupName, v := range hooksMap {
		groups, ok := v.([]interface{})
		if !ok {
			continue
		}

		var kept []interface{}
		hadCabrero := false
		for _, g := range groups {
			raw, _ := json.Marshal(g)
			if strings.Contains(string(raw), "cabrero") {
				hadCabrero = true
				continue
			}
			kept = append(kept, g)
		}

		if hadCabrero {
			removed = append(removed, groupName)
			if len(kept) == 0 {
				delete(hooksMap, groupName)
			} else {
				hooksMap[groupName] = kept
			}
		}
	}

	return removed
}

// Step 5: Remove hook scripts directory.
func (u *uninstallRunner) stepRemoveHookScripts(step, total int) error {
	hooksDir := filepath.Join(store.Root(), "hooks")

	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		fmt.Println("  ✓ Hook scripts not present (already removed)")
		return nil
	}

	if err := os.RemoveAll(hooksDir); err != nil {
		return fmt.Errorf("removing hooks directory: %w", err)
	}

	home, _ := os.UserHomeDir()
	display := strings.Replace(hooksDir, home, "~", 1)
	fmt.Printf("  ✓ Removed %s/\n", display)
	return nil
}

// Step 6: Remove binary directory.
func (u *uninstallRunner) stepRemoveBinary(step, total int) error {
	binDir := filepath.Join(store.Root(), "bin")

	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		fmt.Println("  ✓ Binary not present (already removed)")
		return nil
	}

	if err := os.RemoveAll(binDir); err != nil {
		return fmt.Errorf("removing bin directory: %w", err)
	}

	home, _ := os.UserHomeDir()
	display := strings.Replace(binDir, home, "~", 1)
	fmt.Printf("  ✓ Removed %s/\n", display)
	return nil
}

// Step 7: Data directory prompt.
func (u *uninstallRunner) stepDataDirectory(step, total int) error {
	root := store.Root()
	home, _ := os.UserHomeDir()
	display := strings.Replace(root, home, "~", 1)

	if _, err := os.Stat(root); os.IsNotExist(err) {
		fmt.Printf("  ✓ %s not present (already removed)\n", display)
		return nil
	}

	// Determine action based on flags.
	remove := false
	if u.removeData {
		remove = true
	} else if u.keepData {
		remove = false
	} else if u.autoYes {
		// --yes without explicit data flag defaults to keeping data.
		remove = false
	} else {
		remove = u.promptDataChoice(display)
	}

	if remove {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("removing data directory: %w", err)
		}
		fmt.Printf("  ✓ Removed %s\n", display)
	} else {
		size := dirSize(root)
		fmt.Printf("  ✓ Kept %s (%s)\n", display, formatBytes(size))
	}

	return nil
}

// promptDataChoice presents a numbered choice for the data directory.
// Returns true if user chose to remove.
func (u *uninstallRunner) promptDataChoice(display string) bool {
	fmt.Printf("  %s contains session data, proposals, and prompts.\n", display)
	fmt.Println("  Keep for reinstallation or remove everything?")
	fmt.Println("    [1] Keep (can reinstall later)")
	fmt.Println("    [2] Remove everything")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("  Choice [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	return input == "2"
}

// dirSize calculates the total size of a directory tree in bytes.
func dirSize(path string) int64 {
	var total int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// formatBytes returns a human-readable size string.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return strconv.FormatInt(b, 10) + " B"
	}
}
