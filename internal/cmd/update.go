package cmd

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Update checks for and applies self-updates from GitHub Releases.
func Update(args []string, currentVersion string, hooks EmbeddedHooks) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "check for updates without downloading")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Println("Checking GitHub for latest release...")

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("cannot reach GitHub API: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	fmt.Printf("Latest version:  %s\n", latestVersion)

	if latestVersion == currentVersion {
		fmt.Printf("\nAlready up to date (v%s)\n", currentVersion)
		return nil
	}

	if *checkOnly {
		fmt.Printf("\nUpdate available: %s → %s\n", currentVersion, latestVersion)
		fmt.Println("Run 'cabrero update' to install.")
		return nil
	}

	fmt.Println()

	// Find the correct asset for this platform.
	assetName := fmt.Sprintf("cabrero_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var assetURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (looked for %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	// Download tarball.
	fmt.Printf("Downloading %s...\n", assetName)
	tarballPath, err := downloadToTemp(assetURL)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}
	defer os.Remove(tarballPath)

	tarballSize, _ := fileSize(tarballPath)
	fmt.Printf("  ✓ Downloaded (%.1f MB)\n", float64(tarballSize)/(1024*1024))

	// Verify checksum.
	var checksumURL string
	for _, a := range release.Assets {
		if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
			break
		}
	}
	if checksumURL != "" {
		if err := verifyChecksum(tarballPath, assetName, checksumURL); err != nil {
			return fmt.Errorf("checksum verification failed — aborting: %w", err)
		}
		fmt.Println("  ✓ Checksum verified")
	}

	// Extract binary from tarball.
	tmpDir, err := os.MkdirTemp("", "cabrero-update-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	extractedBinary, err := extractBinary(tarballPath, tmpDir)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	// Replace current binary.
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current binary path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(currentBinary)
	if err == nil {
		currentBinary = resolved
	}

	// Atomic replace: copy to .new then rename.
	newPath := currentBinary + ".new"
	if err := copyFile(extractedBinary, newPath); err != nil {
		return fmt.Errorf("writing new binary: %w\n  Try: sudo cp %s %s", err, extractedBinary, currentBinary)
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(newPath, currentBinary); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("replacing binary: %w\n  Try: sudo cp %s %s", err, extractedBinary, currentBinary)
	}

	// Verify replaced binary runs.
	out, err := exec.Command(currentBinary, "version").Output()
	if err != nil {
		return fmt.Errorf("replaced binary failed to run: %w", err)
	}
	fmt.Printf("  ✓ Binary replaced at %s\n", currentBinary)

	// Post-update: refresh hooks.
	fmt.Println("\nUpdating hooks and configuration...")
	if err := Setup([]string{"--yes", "--hooks-only"}, hooks); err != nil {
		fmt.Printf("  ! Hook update failed: %v\n", err)
	} else {
		fmt.Println("  ✓ Hook scripts updated")
	}

	// Reload LaunchAgent if installed.
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.cabrero.daemon.plist")
		if _, err := os.Stat(plistPath); err == nil {
			exec.Command("launchctl", "unload", plistPath).Run()
			exec.Command("launchctl", "load", plistPath).Run()
			fmt.Println("  ✓ LaunchAgent reloaded")
		}
	}

	newVer := strings.TrimSpace(string(out))
	fmt.Printf("\nUpdated from %s → %s\n", currentVersion, newVer)
	return nil
}

// --- GitHub API types ---

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchLatestRelease() (*ghRelease, error) {
	url := "https://api.github.com/repos/vladolaru/cabrero/releases/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check your connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &release, nil
}

// --- Download helpers ---

func downloadToTemp(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "cabrero-download-*")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

func verifyChecksum(filePath, assetName, checksumURL string) error {
	resp, err := http.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading checksums: %w", err)
	}

	// Parse checksums.txt: each line is "hash  filename".
	var expectedHash string
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			expectedHash = parts[0]
			break
		}
	}
	if expectedHash == "" {
		return fmt.Errorf("no checksum found for %s", assetName)
	}

	// Compute actual hash.
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	if actualHash != expectedHash {
		return fmt.Errorf("expected %s, got %s", expectedHash, actualHash)
	}
	return nil
}

func extractBinary(tarballPath, destDir string) (string, error) {
	f, err := os.Open(tarballPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tarball: %w", err)
		}

		// Look for the cabrero binary (may be at root or in a subdirectory).
		if header.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(header.Name)
		if base != "cabrero" {
			continue
		}

		destPath := filepath.Join(destDir, "cabrero")
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
		return destPath, nil
	}

	return "", fmt.Errorf("cabrero binary not found in tarball")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
