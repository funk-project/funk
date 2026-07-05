package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/funk-project/funk/internal/funk"
)

// cmdServe runs funkd: a local server that executes functions and streams
// results over NDJSON (docs/03 §5 — the CLI is a thin client, the server runs).
func cmdServe(args []string) error {
	files, rest := takeFlag(args, "-f")
	addrs, _ := takeFlag(rest, "--addr")
	addr := ":7777"
	if len(addrs) > 0 {
		addr = addrs[len(addrs)-1]
	}
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}

	mux := serveMux(lib)

	fmt.Fprintf(os.Stderr, "funkd listening on %s — %d functions, %d types\n", addr, len(lib.Fns), len(lib.Types)/2)
	fmt.Fprintf(os.Stderr, "  GET  /health  /functions  /introspect?fn=NAME  /search?q=QUERY\n  POST /run {\"ref\":\"add\",\"inputs\":{\"a\":40,\"b\":2}}\n")
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}

// serveMux builds the funkd HTTP handlers over a library. Kept separate from
// cmdServe so the routes are exercisable with httptest (no real socket).
func serveMux(lib *funk.Library) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/functions", func(w http.ResponseWriter, r *http.Request) {
		type info struct {
			Address string `json:"address"`
			Display string `json:"display"`
			Kind    string `json:"kind"`
			Doc     string `json:"doc"`
		}
		var out []info
		for _, f := range lib.Fns {
			kind := "atomic:" + f.Engine
			if f.Composite() {
				kind = "composite"
			}
			out = append(out, info{f.Address(), f.DisplayName(), kind, f.Doc})
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing ?q=", http.StatusBadRequest)
			return
		}
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				limit = n
			}
		}
		writeJSON(w, searchResults(lib, q, limit))
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, r *http.Request) {
		in, ok := funk.Introspect(lib, r.URL.Query().Get("fn"))
		if !ok {
			http.Error(w, "unknown function", http.StatusNotFound)
			return
		}
		writeJSON(w, in)
	})
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ref    string                 `json:"ref"`
			Inputs map[string]interface{} `json:"inputs"`
			Trace  bool                   `json:"trace"`
			Live   bool                   `json:"live"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		enc := json.NewEncoder(w)
		flush := func() {
			if flusher != nil {
				flusher.Flush()
			}
		}
		// Live: stream per-node trace events (incl. "enter" glow) and values as
		// they happen, then the final report — the substrate for an animated,
		// self-observable trace (docs/01 #4). The plan lights up as it runs.
		if req.Live {
			onEvent := func(ev funk.TraceEvent) {
				_ = enc.Encode(map[string]interface{}{"event": ev})
				flush()
			}
			onValue := func(v interface{}) {
				_ = enc.Encode(map[string]interface{}{"value": v})
				flush()
			}
			_, rep := funk.RunLive(lib, req.Ref, req.Inputs, funk.ExecOpts{}, onEvent, onValue)
			if rep.Error != "" {
				_ = enc.Encode(map[string]interface{}{"error": rep.Error})
			}
			_ = enc.Encode(map[string]interface{}{"report": rep})
			flush()
			return
		}
		if req.Trace {
			res, rep := funk.RunWithReport(lib, req.Ref, req.Inputs, funk.ExecOpts{})
			if !res.OK {
				_ = enc.Encode(map[string]interface{}{"error": res.Error})
			} else {
				_ = enc.Encode(map[string]interface{}{"value": res.Value})
				_ = enc.Encode(map[string]interface{}{"report": rep})
			}
			flush()
			return
		}
		emit := func(v interface{}) {
			_ = enc.Encode(map[string]interface{}{"value": v})
			flush()
		}
		res := funk.RunStreaming(lib, req.Ref, req.Inputs, funk.ExecOpts{}, emit)
		if !res.OK {
			_ = enc.Encode(map[string]interface{}{"error": res.Error})
			flush()
		}
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// runViaServer sends a run to funkd and streams the results back (thin client).
func runViaServer(server, ref string, inputs map[string]interface{}) error {
	body, _ := json.Marshal(map[string]interface{}{"ref": ref, "inputs": inputs})
	resp, err := http.Post(server+"/run", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]interface{}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if e, ok := m["error"]; ok {
			return fmt.Errorf("%v", e)
		}
		if v, ok := m["value"]; ok {
			printValue(v)
		}
	}
	return sc.Err()
}
