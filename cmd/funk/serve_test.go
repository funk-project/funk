package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/funk-project/funk/internal/funk"
)

func serveTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	lib := funk.NewLibrary()
	if err := lib.LoadString(cliLib); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(serveMux(lib))
	t.Cleanup(srv.Close)
	return srv
}

func getBody(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func postBody(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestServeGetEndpoints(t *testing.T) {
	srv := serveTestServer(t)
	if _, body := getBody(t, srv.URL+"/health"); !strings.Contains(body, "ok") {
		t.Fatalf("/health = %q", body)
	}
	if _, body := getBody(t, srv.URL+"/functions"); !strings.Contains(body, "bump") {
		t.Fatalf("/functions = %q", body)
	}
	if code, body := getBody(t, srv.URL+"/introspect?fn=bump"); code != 200 || !strings.Contains(body, "composite") {
		t.Fatalf("/introspect?fn=bump = %d %q", code, body)
	}
	if code, _ := getBody(t, srv.URL+"/introspect?fn=nope"); code != http.StatusNotFound {
		t.Fatalf("/introspect?fn=nope status = %d, want 404", code)
	}
}

func TestServeRunPlainTraceLive(t *testing.T) {
	srv := serveTestServer(t)
	// plain: one {"value":105} NDJSON line
	if code, body := postBody(t, srv.URL+"/run", `{"ref":"bump","inputs":{"x":5}}`); code != 200 || !strings.Contains(body, "105") {
		t.Fatalf("/run plain = %d %q", code, body)
	}
	// trace: value + report
	if _, body := postBody(t, srv.URL+"/run", `{"ref":"bump","inputs":{"x":5},"trace":true}`); !strings.Contains(body, "report") || !strings.Contains(body, "105") {
		t.Fatalf("/run trace = %q", body)
	}
	// live: per-node events + value + report
	if _, body := postBody(t, srv.URL+"/run", `{"ref":"bump","inputs":{"x":5},"live":true}`); !strings.Contains(body, "event") || !strings.Contains(body, "report") {
		t.Fatalf("/run live = %q", body)
	}
	// malformed body → 400
	if code, _ := postBody(t, srv.URL+"/run", `not json`); code != http.StatusBadRequest {
		t.Fatalf("/run bad body status = %d, want 400", code)
	}
}

// runViaServer is the thin client: POST /run and print the streamed values.
func TestRunViaServer(t *testing.T) {
	srv := serveTestServer(t)
	out, err := captureStdout(t, func() error {
		return runViaServer(srv.URL, "bump", map[string]interface{}{"x": 5.0})
	})
	if err != nil || !strings.Contains(out, "105") {
		t.Fatalf("runViaServer: err=%v out=%q", err, out)
	}
	// an unknown ref surfaces the server-side error to the client
	if err := runViaServer(srv.URL, "nope", map[string]interface{}{}); err == nil {
		t.Fatal("runViaServer(nope) should return the server error")
	}
}
