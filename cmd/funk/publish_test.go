package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pubLib is a small green project: packaged, checked, with a passing test.
const pubLib = `package "gen/pubtest" {
  version 0.0.1
}

fn dobro { in (a Num) out (result Num) engine builtin src "num.double"
  test (is (dobro 4) 8) }

fn quadruplo { in (a Num) out (result Num) body (flush (result (dobro (dobro a))))
  test (is (quadruplo 2) 8) }
`

// writePubProject writes a project dir + isolates std/cache/registry to temp.
func writePubProject(t *testing.T, src string) (proj, reg string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FUNK_STD", filepath.Join(dir, "no-std"))
	t.Setenv("FUNK_CACHE", filepath.Join(dir, "no-cache"))
	reg = filepath.Join(dir, "registry")
	t.Setenv("FUNK_REGISTRY", reg)
	proj = filepath.Join(dir, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "main.funk"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj, reg
}

// A green project publishes: files land under the package path, the manifest is
// stamped, the registry has a git commit naming the package, and search finds
// the published fn without -f.
func TestCmdPublishAndSearchRegistry(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	proj, reg := writePubProject(t, pubLib)
	out, err := captureStdout(t, func() error { return cmdPublish([]string{"-f", proj}) })
	if err != nil || !strings.Contains(out, "published gen/pubtest") {
		t.Fatalf("cmdPublish: err=%v out=%q", err, out)
	}
	if _, err := os.Stat(filepath.Join(reg, "gen", "pubtest", "main.funk")); err != nil {
		t.Fatalf("published source missing: %v", err)
	}
	man, err := os.ReadFile(filepath.Join(reg, "gen", "pubtest", "funk-manifest.json"))
	if err != nil || !strings.Contains(string(man), `"gen/pubtest"`) || !strings.Contains(string(man), `"publishedAt"`) {
		t.Fatalf("manifest: err=%v content=%q", err, man)
	}
	log, err := exec.Command("git", "-C", reg, "log", "--oneline").Output()
	if err != nil || !strings.Contains(string(log), "publish gen/pubtest: dobro quadruplo") {
		t.Fatalf("git log: err=%v log=%q", err, log)
	}
	// the commons is searchable: no -f, only the registry provides `dobro`.
	sout, err := captureStdout(t, func() error { return cmdSearch([]string{"dobro"}) })
	if err != nil || !strings.Contains(sout, "gen/pubtest/dobro") {
		t.Fatalf("cmdSearch registry: err=%v out=%q", err, sout)
	}
	// republishing unchanged content is a friendly no-op, not an error.
	out2, err := captureStdout(t, func() error { return cmdPublish([]string{"-f", proj}) })
	if err != nil || !strings.Contains(out2, "unchanged") {
		t.Fatalf("cmdPublish (unchanged): err=%v out=%q", err, out2)
	}
}

// A project with a failing test is refused — nothing lands in the registry.
func TestCmdPublishRefusesFailingTest(t *testing.T) {
	proj, reg := writePubProject(t, strings.Replace(pubLib, "(is (dobro 4) 8)", "(is (dobro 4) 9)", 1))
	_, err := captureStdout(t, func() error { return cmdPublish([]string{"-f", proj}) })
	if err == nil || !strings.Contains(err.Error(), "publish refused") || !strings.Contains(err.Error(), "FAIL dobro") {
		t.Fatalf("expected a test-failure refusal, got %v", err)
	}
	if _, statErr := os.Stat(reg); !os.IsNotExist(statErr) {
		t.Fatalf("registry must stay untouched on refusal")
	}
}

// A project with a check error is refused before tests even matter.
func TestCmdPublishRefusesCheckError(t *testing.T) {
	proj, reg := writePubProject(t, pubLib+`
fn partido { in (valor Num) out (result Num) body (flush (result (naoExiste valor))) }
`)
	_, err := captureStdout(t, func() error { return cmdPublish([]string{"-f", proj}) })
	if err == nil || !strings.Contains(err.Error(), "publish refused") || !strings.Contains(err.Error(), "naoExiste") {
		t.Fatalf("expected a check refusal, got %v", err)
	}
	if _, statErr := os.Stat(reg); !os.IsNotExist(statErr) {
		t.Fatalf("registry must stay untouched on refusal")
	}
}

// An unpackaged project is refused — the package name IS the registry path.
func TestCmdPublishRefusesNoPackage(t *testing.T) {
	proj, _ := writePubProject(t, `fn solto { in (a Num) out (result Num) engine builtin src "num.double" test (is (solto 2) 4) }`)
	_, err := captureStdout(t, func() error { return cmdPublish([]string{"-f", proj}) })
	if err == nil || !strings.Contains(err.Error(), "package block") {
		t.Fatalf("expected a no-package refusal, got %v", err)
	}
}
