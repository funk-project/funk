package funk

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// brokerConfig is a run's brokered capabilities (docs/06 §5): the credentials a
// body may use WITHOUT holding them, and the hosts it may reach.
type brokerConfig struct {
	creds map[string]string // alias → credential (never sent to the body)
	hosts map[string]bool   // allowed egress hosts (from effects { net … })
}

// brokerCall is the request a body makes to the broker instead of calling the
// integration directly: name the capability, not the secret.
type brokerCall struct {
	Secret  string            `json:"secret"` // capability alias, e.g. "gh"
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// startBroker runs the per-run broker on localhost and returns its base URL and
// a stop func. The broker is the body's ONLY path to the network (docs/06 §3):
// it injects the declared credential and enforces the host allowlist, so the
// body can neither read the secret nor reach an undeclared host.
func startBroker(cfg brokerConfig) (string, func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/call", func(w http.ResponseWriter, r *http.Request) {
		var c brokerCall
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "bad broker call", http.StatusBadRequest)
			return
		}
		u, err := url.Parse(c.URL)
		if err != nil || u.Hostname() == "" {
			http.Error(w, "bad url", http.StatusBadRequest)
			return
		}
		// egress control: only declared hosts (docs/06 §7).
		if !cfg.hosts[u.Hostname()] {
			http.Error(w, "egress denied: host "+u.Hostname()+" is not in effects{net}", http.StatusForbidden)
			return
		}
		method := c.Method
		if method == "" {
			method = http.MethodGet
		}
		up, err := http.NewRequest(method, c.URL, bytes.NewReader(c.Body))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		for k, v := range c.Headers {
			up.Header.Set(k, v)
		}
		// credential injection: the body named the capability; we attach the token.
		if tok, ok := cfg.creds[c.Secret]; ok && tok != "" {
			up.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := client.Do(up)
		if err != nil {
			http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return "http://" + ln.Addr().String(), func() { srv.Close() }, nil
}

// brokerConfigFor builds the broker config from a function's resolved resources:
// integration-kind needs become brokered credentials, and effects{net} hosts
// become the egress allowlist. Raw secret/config/env/volume needs are NOT
// brokered — they follow the Phase-0 injection path (docs/06 §6).
func brokerConfigFor(lib *Library, f *Fn, opts ExecOpts) (brokerConfig, bool) {
	cfg := brokerConfig{creds: map[string]string{}, hosts: map[string]bool{}}
	needs, effects := aggregateResources(lib, f, map[string]bool{})
	for _, n := range needs {
		if isRawKind(n.Kind) {
			continue // raw secret/config/… inject directly, not via the broker
		}
		if v, ok := resolveNeed(n, opts).(string); ok && v != "" {
			cfg.creds[n.Alias] = v
		}
	}
	for _, e := range effects {
		if e.Kind == "net" {
			for _, h := range e.Args {
				cfg.hosts[h] = true
			}
		}
	}
	return cfg, len(cfg.creds) > 0
}

// isRawKind reports whether a need is injected raw (not brokered): the built-in
// resource kinds. Anything else is an integration name and is brokered.
func isRawKind(kind string) bool {
	switch kind {
	case "secret", "config", "env", "volume":
		return true
	}
	return false
}
