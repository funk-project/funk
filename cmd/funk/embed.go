package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/funk-project/funk/internal/funk"
)

// Semantic search over an embedding index. The embedding source is any
// OpenAI-compatible `/v1/embeddings` endpoint — OpenAI, or a LOCAL, free one like
// Ollama (`ollama serve`; FUNK_EMBED_URL=http://localhost:11434/v1/embeddings,
// FUNK_EMBED_MODEL=nomic-embed-text). Config via env:
//   FUNK_EMBED_URL    the embeddings endpoint
//   FUNK_EMBED_MODEL  the model name
//   FUNK_EMBED_KEY    optional bearer token (OpenAI; Ollama needs none)
//   FUNK_EMBED_INDEX  the index file (default funk.embeddings.json)

func embedIndexPath() string {
	if p := os.Getenv("FUNK_EMBED_INDEX"); p != "" {
		return p
	}
	return "funk.embeddings.json"
}

// embedTexts sends texts to the configured embeddings endpoint and returns one
// vector per text (in order).
func embedTexts(texts []string) ([][]float64, error) {
	url := os.Getenv("FUNK_EMBED_URL")
	if url == "" {
		return nil, fmt.Errorf("set FUNK_EMBED_URL (e.g. http://localhost:11434/v1/embeddings for a local Ollama) and FUNK_EMBED_MODEL")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model": os.Getenv("FUNK_EMBED_MODEL"),
		"input": texts,
	})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := os.Getenv("FUNK_EMBED_KEY"); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embeddings endpoint returned %s", resp.Status)
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embeddings: got %d vectors for %d inputs", len(out.Data), len(texts))
	}
	vecs := make([][]float64, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// searchText is what a function is indexed/searched by.
func searchText(f *funk.Fn) string {
	return strings.TrimSpace(f.Name + " " + f.Display + " " + f.Doc + " " + f.Examples)
}

type embedEntry struct {
	searchRow
	Vec []float64 `json:"vec"`
}

type embedIndex struct {
	Model   string       `json:"model"`
	Entries []embedEntry `json:"entries"`
}

// scoredRow is a search result carrying its rank score (cosine, for semantic).
type scoredRow struct {
	searchRow
	Score float64 `json:"score,omitempty"`
}

// hasEmbedIndex reports whether a semantic index file exists to search/maintain.
func hasEmbedIndex() bool {
	_, err := os.Stat(embedIndexPath())
	return err == nil
}

// reuseCandidates ranks the library for a task so a generator can compose instead
// of reinventing: semantic when an index + embeddings endpoint are configured,
// otherwise text/signature. The bool reports whether the scores are semantic
// (cosine), so callers can threshold on them (a text score has no such scale).
func reuseCandidates(lib *funk.Library, task string, limit int) ([]scoredRow, bool) {
	if hasEmbedIndex() && os.Getenv("FUNK_EMBED_URL") != "" {
		if rows, err := searchSemanticScored(task, limit); err == nil {
			return rows, true
		} else {
			fmt.Fprintf(os.Stderr, "· semantic search unavailable (%v) — using text search\n", err)
		}
	}
	syn := searchResults(lib, task, limit)
	out := make([]scoredRow, len(syn))
	for i, r := range syn {
		out[i] = scoredRow{searchRow: r}
	}
	return out, false
}

// indexUpsert refreshes the semantic index for one function after it is written,
// so a freshly generated fn is immediately discoverable (incremental reindex). It
// is a silent no-op when there is no index / embeddings endpoint to maintain.
func indexUpsert(lib *funk.Library, name string) error {
	if !hasEmbedIndex() || os.Getenv("FUNK_EMBED_URL") == "" {
		return nil
	}
	f, ok := lib.Lookup(name)
	if !ok {
		return fmt.Errorf("indexUpsert: no function %q", name)
	}
	b, err := os.ReadFile(embedIndexPath())
	if err != nil {
		return err
	}
	var idx embedIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return err
	}
	vecs, err := embedTexts([]string{searchText(f)})
	if err != nil {
		return err
	}
	entry := embedEntry{
		searchRow: searchRow{f.Address(), f.Name, f.Display, signature(f), docSummary(f.Doc)},
		Vec:       vecs[0],
	}
	replaced := false
	for i := range idx.Entries {
		if idx.Entries[i].Address == entry.Address {
			idx.Entries[i], replaced = entry, true
			break
		}
	}
	if !replaced {
		idx.Entries = append(idx.Entries, entry)
	}
	nb, _ := json.Marshal(idx)
	if err := os.WriteFile(embedIndexPath(), nb, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "· reindexed %s (%d in index)\n", entry.Address, len(idx.Entries))
	return nil
}

// cmdIndex embeds every loaded function and writes the semantic index.
func cmdIndex(args []string) error {
	files, _ := takeFlag(args, "-f")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}
	texts := make([]string, len(lib.Fns))
	for i, f := range lib.Fns {
		texts[i] = searchText(f)
	}
	vecs, err := embedTexts(texts)
	if err != nil {
		return err
	}
	idx := embedIndex{Model: os.Getenv("FUNK_EMBED_MODEL")}
	for i, f := range lib.Fns {
		idx.Entries = append(idx.Entries, embedEntry{
			searchRow: searchRow{f.Address(), f.Name, f.Display, signature(f), docSummary(f.Doc)},
			Vec:       vecs[i],
		})
	}
	b, _ := json.Marshal(idx)
	if err := os.WriteFile(embedIndexPath(), b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "indexed %d functions → %s\n", len(idx.Entries), embedIndexPath())
	return nil
}

// searchSemanticScored embeds the query and ranks the index by cosine similarity,
// keeping each row's score.
func searchSemanticScored(query string, limit int) ([]scoredRow, error) {
	b, err := os.ReadFile(embedIndexPath())
	if err != nil {
		return nil, fmt.Errorf("no index at %s — run `funk index` first (%w)", embedIndexPath(), err)
	}
	var idx embedIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, err
	}
	qv, err := embedTexts([]string{query})
	if err != nil {
		return nil, err
	}
	ranked := make([]scoredRow, len(idx.Entries))
	for i, e := range idx.Entries {
		ranked[i] = scoredRow{e.searchRow, cosine(qv[0], e.Vec)}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

// searchSemantic embeds the query and ranks the index by cosine similarity.
func searchSemantic(query string, limit int) ([]searchRow, error) {
	scored, err := searchSemanticScored(query, limit)
	if err != nil {
		return nil, err
	}
	rows := make([]searchRow, len(scored))
	for i, s := range scored {
		rows[i] = s.searchRow
	}
	return rows, nil
}
