package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cacheDir is the shared package cache (docs/03 §6): ~/.funk/pkg.
func cacheDir() string {
	if d := os.Getenv("FUNK_CACHE"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".funk", "pkg")
}

// cmdGet fetches a package (a git repo — local path or URL) into the cache, so
// its functions resolve like std. Go-style: a package is a git repo, addressed
// by path (docs/03 §6).
func cmdGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("get: usage: funk get <git-url-or-path> [name]")
	}
	url := args[0]
	name := deriveName(url)
	if len(args) > 1 {
		name = args[1]
	}
	dst := filepath.Join(cacheDir(), name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		// already present — update
		out, err := exec.Command("git", "-C", dst, "pull", "--ff-only").CombinedOutput()
		if err != nil {
			return fmt.Errorf("get: update failed: %s", strings.TrimSpace(string(out)))
		}
		fmt.Fprintf(os.Stderr, "updated %s → %s\n", name, dst)
		return nil
	}
	out, err := exec.Command("git", "clone", "--depth", "1", url, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("get: clone failed: %s", strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(os.Stderr, "fetched %s → %s\n", name, dst)
	return nil
}

func deriveName(url string) string {
	u := strings.TrimSuffix(url, ".git")
	u = strings.TrimSuffix(u, "/")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		return u[i+1:]
	}
	return u
}
