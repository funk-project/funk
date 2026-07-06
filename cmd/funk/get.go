package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/funk-project/funk/internal/funk"
)

// cacheDir is the shared package cache (docs/03 §6): ~/.funk/pkg.
func cacheDir() string {
	if d := os.Getenv("FUNK_CACHE"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".funk", "pkg")
}

// splitRef separates a trailing `@ref` (tag / branch / commit) from a git URL,
// WITHOUT mistaking the `@` in an scp-style URL like `git@github.com:u/r.git`.
// A pin is only recognised when the `@` falls after the last `/` (i.e. in the
// repo/path part, never in the user@host part).
func splitRef(url string) (repo, ref string) {
	slash := strings.LastIndex(url, "/")
	at := strings.LastIndex(url, "@")
	if slash >= 0 && at > slash {
		return url[:at], url[at+1:]
	}
	return url, ""
}

// cmdGet fetches a package (a git repo — local path or URL) into the cache, so
// its functions resolve like std. Go-style: a package is a git repo, addressed
// by path (docs/03 §6). A trailing `@ref` pins a tag/branch/commit — the first
// step toward reproducible dependencies (docs/ROADMAP.md).
func cmdGet(args []string) error {
	if len(args) == 0 {
		return getManifest() // read versioned `use` deps from the cwd's .funk files
	}
	name := ""
	if len(args) > 1 {
		name = args[1]
	}
	return getOne(args[0], name)
}

// getOne fetches a single package from urlArg (a git URL/path with an optional
// @ref or version constraint) into cache/<name>, recording a lockfile hash.
func getOne(urlArg, name string) error {
	url, ref := splitRef(urlArg)
	if isConstraint(ref) { // resolve a partial `v1` / `latest` to the best tag
		resolved, err := resolveVersion(url, ref)
		if err != nil {
			return fmt.Errorf("get: %w", err)
		}
		ref = resolved
	}
	if name == "" {
		name = deriveName(url)
	}
	dst := filepath.Join(cacheDir(), name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tag := name
	if ref != "" {
		tag = name + "@" + ref
	}

	if _, err := os.Stat(dst); err == nil {
		// already present — update (checkout the pinned ref, or fast-forward)
		if ref != "" {
			exec.Command("git", "-C", dst, "fetch", "--tags", "origin").Run()
			if out, err := exec.Command("git", "-C", dst, "checkout", ref).CombinedOutput(); err != nil {
				return fmt.Errorf("get: checkout %s failed: %s", ref, strings.TrimSpace(string(out)))
			}
			return recordLock(dst, name, ref, fmt.Sprintf("updated %s → %s", tag, dst))
		}
		out, err := exec.Command("git", "-C", dst, "pull", "--ff-only").CombinedOutput()
		if err != nil {
			return fmt.Errorf("get: update failed: %s", strings.TrimSpace(string(out)))
		}
		return recordLock(dst, name, ref, fmt.Sprintf("updated %s → %s", name, dst))
	}

	if ref != "" {
		// try a shallow clone of the ref (works for tags/branches); on failure
		// (e.g. a commit SHA) fall back to a full clone + checkout.
		if out, err := exec.Command("git", "clone", "--depth", "1", "--branch", ref, url, dst).CombinedOutput(); err != nil {
			os.RemoveAll(dst)
			if out2, err2 := exec.Command("git", "clone", url, dst).CombinedOutput(); err2 != nil {
				return fmt.Errorf("get: clone failed: %s", strings.TrimSpace(string(out2)))
			}
			if out3, err3 := exec.Command("git", "-C", dst, "checkout", ref).CombinedOutput(); err3 != nil {
				return fmt.Errorf("get: checkout %s failed: %s", ref, strings.TrimSpace(string(out3)))
			}
			_ = out
		}
		return recordLock(dst, name, ref, fmt.Sprintf("fetched %s → %s", tag, dst))
	}

	out, err := exec.Command("git", "clone", "--depth", "1", url, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("get: clone failed: %s", strings.TrimSpace(string(out)))
	}
	return recordLock(dst, name, ref, fmt.Sprintf("fetched %s → %s", name, dst))
}

// recordLock prints the fetch message and, for a version tag, records the
// package's content hash in the lockfile (go.sum-style reproducibility).
func recordLock(dst, name, ref, msg string) error {
	fmt.Fprintln(os.Stderr, msg)
	if _, ok := parseSemver(ref); ok {
		h, err := hashPkg(dst)
		if err != nil {
			return err
		}
		return writeLockEntry(lockPath(), name+"@"+ref, h)
	}
	return nil
}

// getManifest fetches every versioned `use "url" "vX.Y.Z" as alias` dependency
// declared in the current directory's .funk files (a manifest-driven install).
func getManifest() error {
	files, _ := filepath.Glob("*.funk")
	if len(files) == 0 {
		return fmt.Errorf("get: no url given and no .funk files in the current directory")
	}
	seen := map[string]bool{}
	n := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		prog, err := funk.Parse(string(src))
		if err != nil {
			continue
		}
		for _, u := range funk.Uses(prog) {
			if u.Version == "" || seen[u.Pkg+"@"+u.Version] {
				continue
			}
			seen[u.Pkg+"@"+u.Version] = true
			if err := getOne(u.Pkg+"@"+u.Version, ""); err != nil {
				return err
			}
			n++
		}
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "get: no versioned `use` dependencies found")
	}
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
