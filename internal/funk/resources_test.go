package funk

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// resolveNeed prefers an explicit --bind, then the FUNK_<KIND>_<ALIAS> env var,
// then "" (docs/05).
func TestResolveNeed(t *testing.T) {
	n := Need{Kind: "config", Alias: "repo"}
	if v := resolveNeed(n, ExecOpts{Bindings: map[string]string{"config.repo": "bound"}}); v != "bound" {
		t.Fatalf("binding = %v, want bound", v)
	}
	t.Setenv("FUNK_CONFIG_REPO", "fromenv")
	if v := resolveNeed(n, ExecOpts{}); v != "fromenv" {
		t.Fatalf("env fallback = %v, want fromenv", v)
	}
	// an explicit binding wins over the env var
	if v := resolveNeed(n, ExecOpts{Bindings: map[string]string{"config.repo": "bound"}}); v != "bound" {
		t.Fatalf("binding should beat env, got %v", v)
	}
	if v := resolveNeed(Need{Kind: "config", Alias: "absent"}, ExecOpts{}); v != "" {
		t.Fatalf("unset need = %v, want empty", v)
	}
}

func TestResolveResources(t *testing.T) {
	f := &Fn{Needs: []Need{
		{Kind: "config", Alias: "repo"},
		{Kind: "env", Alias: "home"},
		{Kind: "secret", Alias: "token"},
		{Kind: "volume", Alias: "data"},
	}}
	res := resolveResources(f, ExecOpts{Bindings: map[string]string{
		"config.repo":  "acme",
		"env.home":     "/home/x",
		"secret.token": "s3cr3t",
		"volume.data":  "/mnt/data",
	}})
	if res["config"]["repo"] != "acme" || res["env"]["home"] != "/home/x" ||
		res["secret"]["token"] != "s3cr3t" || res["volume"]["data"] != "/mnt/data" {
		t.Fatalf("resolveResources = %#v", res)
	}
	// a fn with no needs resolves to nil
	if resolveResources(&Fn{}, ExecOpts{}) != nil {
		t.Fatal("no-needs fn should resolve to nil")
	}
}

// needsJSON hands the body only RAW needs (secret/config/env/volume); brokered
// integration credentials are reached via the broker, never injected (docs/06 §6).
func TestNeedsJSONDropsBrokered(t *testing.T) {
	f := &Fn{Needs: []Need{
		{Kind: "config", Alias: "repo"},
		{Kind: "github", Alias: "gh"}, // an integration ⇒ brokered, not injected
	}}
	js := needsJSON(f, ExecOpts{Bindings: map[string]string{"config.repo": "acme", "github.gh": "tok"}})
	if !strings.Contains(js, "config") || !strings.Contains(js, "acme") {
		t.Fatalf("needsJSON should carry the raw config need: %s", js)
	}
	if strings.Contains(js, "github") || strings.Contains(js, "tok") {
		t.Fatalf("needsJSON must NOT carry the brokered integration: %s", js)
	}
}

// A funk body reads an injected resource via needs.<kind>.<alias>. Every raw kind
// (config/env/secret/volume) flows through the same path.
func TestFunkBodyResourceAccess(t *testing.T) {
	cases := []struct{ decl, access, bind, val string }{
		{"config repo Str", "needs.config.repo", "config.repo", "acme"},
		{"env home Str", "needs.env.home", "env.home", "/home/x"},
		{"secret token", "needs.secret.token", "secret.token", "s3cr3t"},
		{"volume data", "needs.volume.data", "volume.data", "/mnt/data"},
	}
	for _, c := range cases {
		lib := NewLibrary()
		src := "package \"t/r\" {\n  version 0.0.1\n}\nfn get {\n  in () out (r Str)\n  needs { " +
			c.decl + " }\n  body (flush (r " + c.access + "))\n}"
		if err := lib.LoadString(src); err != nil {
			t.Fatalf("%s: load: %v", c.decl, err)
		}
		res := Run(lib, "get", nil, ExecOpts{Bindings: map[string]string{c.bind: c.val}})
		if !res.OK || res.Value != c.val {
			t.Fatalf("%s: got %v (%s), want %q", c.decl, res.Value, res.Error, c.val)
		}
	}
}

// An unresolved needs.<kind>.<alias> yields "" rather than erroring.
func TestUnknownNeedIsEmpty(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "t/r" {
  version 0.0.1
}
fn get { in () out (r Str) needs { config repo Str } body (flush (r needs.config.missing)) }`); err != nil {
		t.Fatal(err)
	}
	res := Run(lib, "get", nil, ExecOpts{Bindings: map[string]string{"config.repo": "acme"}})
	if !res.OK || res.Value != "" {
		t.Fatalf("unknown need = %v (%s), want empty", res.Value, res.Error)
	}
}

// A secret's value is masked to "***" in the self-observable trace (docs/06 §6),
// but the function's actual return value is left intact for the caller.
func TestSecretRedactedInTrace(t *testing.T) {
	lib := NewLibrary()
	// `id` is a composite passthrough: its call event carries the secret value,
	// which must be redacted in the trace.
	if err := lib.LoadString(`package "t/r" {
  version 0.0.1
}
fn id   { in (x Str) out (r Str) body (flush (r x)) }
fn leak { in () out (r Str) needs { secret token } body (flush (r (id needs.secret.token))) }`); err != nil {
		t.Fatal(err)
	}
	res, rep := RunWithReport(lib, "leak", nil, ExecOpts{Bindings: map[string]string{"secret.token": "hunter2"}})
	if !res.OK || res.Value != "hunter2" {
		t.Fatalf("return value = %v (%s), want intact hunter2", res.Value, res.Error)
	}
	evs, _ := json.Marshal(rep.Events)
	if strings.Contains(string(evs), "hunter2") {
		t.Fatalf("secret leaked into the trace: %s", evs)
	}
	if !strings.Contains(string(evs), "***") {
		t.Fatalf("expected a redacted value in the trace: %s", evs)
	}
}

// End-to-end: a python body reads injected config + secret via needs['kind']['alias'].
func TestPythonNeedsInjection(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	lib := NewLibrary()
	if err := lib.LoadString(`package "t/r" {
  version 0.0.1
}
fn peek {
  in () out (r Str)
  engine python
  needs {
    config repo Str
    secret token
  }
  src "return needs['config']['repo'] + '/' + needs['secret']['token']"
}`); err != nil {
		t.Fatal(err)
	}
	res := Run(lib, "peek", nil, ExecOpts{Bindings: map[string]string{"config.repo": "acme", "secret.token": "s3cr3t"}})
	if !res.OK || res.Value != "acme/s3cr3t" {
		t.Fatalf("python needs injection = %v (%s), want acme/s3cr3t", res.Value, res.Error)
	}
}
