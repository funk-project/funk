package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCosine(t *testing.T) {
	if c := cosine([]float64{1, 0}, []float64{1, 0}); c != 1 {
		t.Fatalf("cosine(self) = %v, want 1", c)
	}
	if c := cosine([]float64{1, 0}, []float64{0, 1}); c != 0 {
		t.Fatalf("cosine(orthogonal) = %v, want 0", c)
	}
	if cosine([]float64{1}, []float64{1, 2}) != 0 {
		t.Fatal("mismatched dims should be 0")
	}
}

// mockEmbed returns a keyword-presence vector for each input, so the pipeline
// (index → embed → cosine → rank) is verifiable without a real model.
func mockEmbed(t *testing.T, vocab []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		type vec struct {
			Embedding []float64 `json:"embedding"`
		}
		out := struct {
			Data []vec `json:"data"`
		}{}
		for _, in := range req.Input {
			v := make([]float64, len(vocab))
			for i, kw := range vocab {
				if strings.Contains(strings.ToLower(in), kw) {
					v[i] = 1
				}
			}
			out.Data = append(out.Data, vec{v})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSemanticIndexAndSearch(t *testing.T) {
	srv := mockEmbed(t, []string{"average", "multiply", "greater"})
	t.Setenv("FUNK_EMBED_URL", srv.URL)
	t.Setenv("FUNK_EMBED_MODEL", "mock")
	t.Setenv("FUNK_EMBED_INDEX", filepath.Join(t.TempDir(), "e.json"))

	p := writeFunk(t, `package "s" {
  version 0.0.1
}
fn mul  { name "Multiplication" doc "multiply two numbers" in (a Num) (b Num) out (r Num) engine builtin src "num.mul" }
fn mean { name "Mean" doc "average of a list" in (xs (List Num)) out (r Num) engine builtin src "list.first" }
fn gt   { name "Greater Than" doc "greater than test" in (a Num) (b Num) out (r Bool) engine builtin src "num.gt" }`)

	if err := cmdIndex([]string{"-f", p}); err != nil {
		t.Fatalf("index: %v", err)
	}
	// "average" is semantically nearest mean (its doc has "average")
	rows, err := searchSemantic("average of numbers", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].Name != "mean" {
		t.Fatalf("semantic 'average' top = %+v, want mean", rows)
	}
	// "multiply" ranks mul first
	rows, _ = searchSemantic("multiply things", 3)
	if len(rows) == 0 || rows[0].Name != "mul" {
		t.Fatalf("semantic 'multiply' top = %+v, want mul", rows)
	}
}

func TestSemanticNoIndex(t *testing.T) {
	t.Setenv("FUNK_EMBED_INDEX", filepath.Join(t.TempDir(), "missing.json"))
	if _, err := searchSemantic("x", 5); err == nil {
		t.Fatal("expected an error when the index is missing")
	}
}
