package funk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecResult is the outcome of running one function body.
type ExecResult struct {
	OK    bool        `json:"ok"`
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

// ExecOpts configures execution (sandbox, timeout, resource bindings).
type ExecOpts struct {
	Sandbox   string            // "" (host) | "docker"
	Timeout   time.Duration
	Bindings  map[string]string // "kind.alias" → value, for needs resolution
	BrokerURL string            // per-run broker base URL (docs/06); "" ⇒ none
}

// resolveResources builds the injected `needs` tree (kind → alias → value) from
// a function's declarations, resolved from --bind bindings then env
// (FUNK_<KIND>_<ALIAS>). This is the runtime injection of docs/05 (v1: direct
// values; brokering/vault is future work).
func resolveResources(f *Fn, opts ExecOpts) map[string]map[string]interface{} {
	if len(f.Needs) == 0 {
		return nil
	}
	res := map[string]map[string]interface{}{}
	for _, n := range f.Needs {
		if res[n.Kind] == nil {
			res[n.Kind] = map[string]interface{}{}
		}
		res[n.Kind][n.Alias] = resolveNeed(n, opts)
	}
	return res
}

func resolveNeed(n Need, opts ExecOpts) interface{} {
	if v, ok := opts.Bindings[n.Kind+"."+n.Alias]; ok {
		return v
	}
	env := "FUNK_" + strings.ToUpper(n.Kind) + "_" + strings.ToUpper(n.Alias)
	if v := os.Getenv(env); v != "" {
		return v
	}
	return ""
}

func needsJSON(f *Fn, opts ExecOpts) string {
	r := resolveResources(f, opts)
	if r == nil {
		return "{}"
	}
	// Brokered integration credentials are reached via the broker, never handed
	// to the body (docs/06 §6): drop non-raw kinds from what the body receives.
	for kind := range r {
		if !isRawKind(kind) {
			delete(r, kind)
		}
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func (o ExecOpts) timeout(def time.Duration) time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return def
}

// Exec runs a single atomic function body with the given named inputs.
// The calling convention: inputs are JSON; the return value is the output.
func Exec(f *Fn, in map[string]interface{}, opts ExecOpts) ExecResult {
	switch f.Engine {
	case "builtin", "":
		return execBuiltin(f, in)
	case "python":
		return execPython(f, in, opts)
	case "go":
		return execGo(f, in, opts)
	case "claude":
		return execClaude(f, in, opts)
	case "codex":
		return execCodex(f, in, opts)
	default:
		return ExecResult{Error: "unknown engine: " + f.Engine}
	}
}

func execBuiltin(f *Fn, in map[string]interface{}) ExecResult {
	p, ok := builtins[f.Src]
	if !ok {
		return ExecResult{Error: fmt.Sprintf("no builtin primitive %q", f.Src)}
	}
	v, err := p(in)
	if err != nil {
		return ExecResult{Error: err.Error()}
	}
	return ExecResult{OK: true, Value: v}
}

func inputsJSON(in map[string]interface{}) string {
	if in == nil {
		in = map[string]interface{}{}
	}
	b, _ := json.Marshal(in)
	return string(b)
}

// runCmd runs a subprocess with a hard timeout; the context kills it on expiry
// (docs/03: exec.CommandContext kills the subprocess on scope-cancel).
func runCmd(name string, args, env []string, timeout time.Duration) ExecResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return ExecResult{Error: fmt.Sprintf("timeout after %s", timeout)}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return ExecResult{Error: fmt.Sprintf("engine binary %q not found in PATH — install it or pick another engine", name)}
	}
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return ExecResult{Error: msg}
	}
	return ExecResult{OK: true, Value: parseOut(strings.TrimSpace(out.String()))}
}

// parseOut tries to decode JSON output; otherwise returns the raw string.
func parseOut(s string) interface{} {
	if s == "" {
		return ""
	}
	var v interface{}
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

func baseEnv(inJSON string) []string {
	env := os.Environ()
	home := os.Getenv("HOME")
	env = append(env,
		"PATH="+home+"/.local/bin:"+home+"/.npm-global/bin:"+os.Getenv("PATH"),
		"FUNK_INPUTS="+inJSON,
	)
	return env
}

func execPython(f *Fn, in map[string]interface{}, opts ExecOpts) ExecResult {
	inJSON := inputsJSON(in)
	var keys []string
	for k := range in {
		if isIdent(k) {
			keys = append(keys, k)
		}
	}
	params := strings.Join(mapStr(keys, func(k string) string { return k + "=None" }), ", ")
	keyList := strings.Join(mapStr(keys, func(k string) string { return "'" + k + "'" }), ", ")
	indented := indentLines(orDefault(f.Src, "return None"), "    ")
	wrapper := fmt.Sprintf(`import sys, json, os
raw = json.loads(sys.argv[1]) if len(sys.argv) > 1 else {}
needs = json.loads(os.environ.get("FUNK_NEEDS", "{}"))
def _p(v):
    try:
        return json.loads(v)
    except Exception:
        return v
_in = {k: _p(v) for k, v in raw.items() if k in [%s]}
def _fn(%s):
%s
_out = _fn(**_in)
sys.stdout.write(_out if isinstance(_out, str) else json.dumps(_out))
`, keyList, params, indented)

	nJSON := needsJSON(f, opts)
	env := append(baseEnv(inJSON), "FUNK_NEEDS="+nJSON)
	if opts.BrokerURL != "" {
		env = append(env, "FUNK_BROKER="+opts.BrokerURL)
	}
	if opts.Sandbox == "docker" {
		image := "python:3-slim"
		if imageExists("funk-py:latest") {
			image = "funk-py:latest"
		}
		// `-e FUNK_NEEDS` (no value) forwards it from the subprocess env, keeping
		// the secret off the docker command line (visible via `ps`/`docker inspect`).
		dargs := []string{"run", "--rm", "-e", "FUNK_NEEDS"}
		// Egress default-deny (docs/06 §7): a body with no declared `net` effect
		// gets no network at all — the raw-secret tier's guarantee (docs/06 §6).
		if !hasNetEffect(f) {
			dargs = append(dargs, "--network", "none")
		}
		dargs = append(dargs, image, "python3", "-c", wrapper, inJSON)
		return runCmd("docker", dargs, env, opts.timeout(180*time.Second))
	}
	return runCmd("python3", []string{"-c", wrapper, inJSON}, env, opts.timeout(30*time.Second))
}

func execGo(f *Fn, in map[string]interface{}, opts ExecOpts) ExecResult {
	inJSON := inputsJSON(in)
	body := indentLines(orDefault(f.Src, "return nil"), "\t")
	program := fmt.Sprintf(`package main
import ("encoding/json"; "fmt"; "os")
func run(in map[string]interface{}) interface{} {
%s
}
func main() {
	in := map[string]interface{}{}
	if len(os.Args) > 1 { json.Unmarshal([]byte(os.Args[1]), &in) }
	for k, v := range in {
		if s, ok := v.(string); ok {
			var p interface{}
			if json.Unmarshal([]byte(s), &p) == nil { in[k] = p }
		}
	}
	out := run(in)
	if s, ok := out.(string); ok { fmt.Print(s) } else { b, _ := json.Marshal(out); fmt.Print(string(b)) }
}
`, body)
	file, err := os.CreateTemp("", "funk_*.go")
	if err != nil {
		return ExecResult{Error: err.Error()}
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(program); err != nil {
		return ExecResult{Error: err.Error()}
	}
	file.Close()
	return runCmd("go", []string{"run", file.Name(), inJSON}, baseEnv(inJSON), opts.timeout(60*time.Second))
}

func execClaude(f *Fn, in map[string]interface{}, opts ExecOpts) ExecResult {
	inJSON := inputsJSON(in)
	prompt := fmt.Sprintf("%s\n\nInputs (JSON): %s\n\nRespond with ONLY the result value (no prose).", f.Src, inJSON)
	return runCmd("claude", []string{"-p", "--model", "sonnet", prompt}, baseEnv(inJSON), opts.timeout(120*time.Second))
}

func execCodex(f *Fn, in map[string]interface{}, opts ExecOpts) ExecResult {
	inJSON := inputsJSON(in)
	prompt := fmt.Sprintf("%s\n\nInputs (JSON): %s\n\nRespond with ONLY the result value (no prose).", f.Src, inJSON)
	return runCmd("codex", []string{"exec", prompt}, baseEnv(inJSON), opts.timeout(120*time.Second))
}

// hasNetEffect reports whether f declares any `net` egress capability. A body
// without one runs with no network (docs/06 §7, egress default-deny).
func hasNetEffect(f *Fn) bool {
	for _, e := range f.Effects {
		if e.Kind == "net" {
			return true
		}
	}
	return false
}

func imageExists(img string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "image", "inspect", img).Run() == nil
}
