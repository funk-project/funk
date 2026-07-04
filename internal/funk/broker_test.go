package funk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// The broker is the body's only path to the network: for a declared host it
// proxies the call and injects the named credential (the body never sees it).
func TestBrokerProxiesAndInjectsCredential(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		io.WriteString(w, "upstream-ok")
	}))
	defer upstream.Close()
	host := must(url.Parse(upstream.URL)).Hostname()

	cfg := brokerConfig{creds: map[string]string{"gh": "tok123"}, hosts: map[string]bool{host: true}}
	base, stop, err := startBroker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	body, _ := json.Marshal(brokerCall{Secret: "gh", URL: upstream.URL, Method: "GET"})
	resp := post(t, base+"/call", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if b, _ := io.ReadAll(resp.Body); string(b) != "upstream-ok" {
		t.Fatalf("proxied body = %q", b)
	}
	if gotAuth != "Bearer tok123" {
		t.Fatalf("injected Authorization = %q, want Bearer tok123", gotAuth)
	}
}

// A host not in effects{net} is refused (egress control, docs/06 §7).
func TestBrokerEgressDenied(t *testing.T) {
	base, stop, err := startBroker(brokerConfig{creds: map[string]string{}, hosts: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	body, _ := json.Marshal(brokerCall{URL: "http://example.com/x"})
	resp := post(t, base+"/call", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (egress denied)", resp.StatusCode)
	}
}

func TestBrokerBadRequests(t *testing.T) {
	base, stop, err := startBroker(brokerConfig{creds: map[string]string{}, hosts: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if resp := post(t, base+"/call", "not json"); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("bad json status = %d, want 400", resp.StatusCode)
	}
	body, _ := json.Marshal(brokerCall{URL: "://bad"})
	if resp := post(t, base+"/call", string(body)); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("bad url status = %d, want 400", resp.StatusCode)
	}
}

// brokerConfigFor brokers integration-kind needs + net hosts, but NOT raw
// secret/config needs (those inject directly).
func TestBrokerConfigFor(t *testing.T) {
	lib := NewLibrary()
	if err := lib.LoadString(`package "t/broker" {
  version 0.0.1
}
fn ghCall  { in () out (r Str) engine python needs { github gh } effects { net "api.github.com" } src "return '1'" }
fn rawOnly { in () out (r Str) engine python needs { secret token } src "return '1'" }`); err != nil {
		t.Fatal(err)
	}
	gh, _ := lib.Lookup("ghCall")
	cfg, ok := brokerConfigFor(lib, gh, ExecOpts{Bindings: map[string]string{"github.gh": "tok"}})
	if !ok {
		t.Fatal("ghCall should be brokered (integration need present)")
	}
	if cfg.creds["gh"] != "tok" {
		t.Fatalf("creds[gh] = %q, want tok", cfg.creds["gh"])
	}
	if !cfg.hosts["api.github.com"] {
		t.Fatalf("hosts = %v, want api.github.com allowed", cfg.hosts)
	}
	raw, _ := lib.Lookup("rawOnly")
	if _, ok := brokerConfigFor(lib, raw, ExecOpts{Bindings: map[string]string{"secret.token": "s"}}); ok {
		t.Fatal("a raw secret need must NOT be brokered")
	}
}

func TestIsRawKind(t *testing.T) {
	for _, k := range []string{"secret", "config", "env", "volume"} {
		if !isRawKind(k) {
			t.Errorf("isRawKind(%q) = false, want true", k)
		}
	}
	if isRawKind("github") {
		t.Error("isRawKind(github) = true, want false (integration is brokered)")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
