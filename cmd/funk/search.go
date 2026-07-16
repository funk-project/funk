package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/funk-project/funk/internal/funk"
)

// cmdSearch finds existing functions so an author (human or AI) can reuse instead
// of reinventing. Two modes: a signature query (`Num Num -> Bool`) matches by
// input/output types; anything else is a text query scored over name, display
// label, doc and examples.
func cmdSearch(args []string) error {
	files, rest := takeFlag(args, "-f")
	asJSON, rest := takeBool(rest, "--json")
	semantic, rest := takeBool(rest, "--semantic")
	limits, rest := takeFlag(rest, "--limit")
	limit := 20
	if len(limits) > 0 {
		if n, err := strconv.Atoi(limits[len(limits)-1]); err == nil {
			limit = n
		}
	}
	if len(rest) == 0 {
		return fmt.Errorf("search: usage: funk search [--json] [--semantic] [--limit N] <query | in… -> out>")
	}
	query := strings.Join(rest, " ")

	var rows []searchRow
	if semantic { // embedding-ranked; needs an index (funk index) + FUNK_EMBED_URL
		var err error
		if rows, err = searchSemantic(query, limit); err != nil {
			return err
		}
	} else {
		lib, err := loadLibrary(files)
		if err != nil {
			return err
		}
		// the commons: published funktions (funk publish) are searchable too.
		// Best-effort and additive — a missing or broken registry never breaks search.
		if reg := registryDir(""); reg != "" {
			if fi, err := os.Stat(reg); err == nil && fi.IsDir() {
				_ = lib.LoadDir(reg)
				_ = lib.Finalize()
			}
		}
		rows = searchResults(lib, query, limit)
	}
	if asJSON {
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if len(rows) == 0 {
		fmt.Println("no matches")
		return nil
	}
	for _, r := range rows {
		disp := ""
		if r.Display != "" && r.Display != r.Name {
			disp = "  “" + r.Display + "”"
		}
		fmt.Printf("%s%s\n    %s\n    %s\n", r.Address, disp, r.Signature, r.Doc)
	}
	return nil
}

// searchRow is one result of a library search (shared by `funk search` and funkd's
// /search endpoint).
type searchRow struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Display   string `json:"display,omitempty"`
	Signature string `json:"signature"`
	Doc       string `json:"doc,omitempty"`
}

// searchResults scores every function against the query (signature mode when the
// query has an arrow, else text mode), ranks, and truncates to limit.
func searchResults(lib *funk.Library, query string, limit int) []searchRow {
	type hit struct {
		f     *funk.Fn
		score int
	}
	var hits []hit
	if wantIn, wantOut, ok := splitArrow(query); ok {
		for _, f := range lib.Fns {
			if s := sigScore(f, wantIn, wantOut); s > 0 {
				hits = append(hits, hit{f, s})
			}
		}
	} else {
		toks := strings.Fields(strings.ToLower(query))
		for _, f := range lib.Fns {
			if s := textScore(f, toks); s > 0 {
				hits = append(hits, hit{f, s})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].f.Address() < hits[j].f.Address()
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	rows := make([]searchRow, 0, len(hits))
	seen := map[string]bool{} // a registry copy of a loaded package would double-list
	for _, h := range hits {
		if seen[h.f.Address()] {
			continue
		}
		seen[h.f.Address()] = true
		rows = append(rows, searchRow{h.f.Address(), h.f.Name, h.f.Display, signature(h.f), docSummary(h.f.Doc)})
	}
	return rows
}

// splitArrow splits a signature query `A B -> C` into its input types and output
// type; ok is false when there is no arrow.
func splitArrow(q string) (in []string, out string, ok bool) {
	sep := ""
	switch {
	case strings.Contains(q, "->"):
		sep = "->"
	case strings.Contains(q, "→"):
		sep = "→"
	default:
		return nil, "", false
	}
	parts := strings.SplitN(q, sep, 2)
	return strings.Fields(parts[0]), strings.TrimSpace(parts[1]), true
}

// typeBase lowercases a type and strips any `<…>` parameter, so `List<Num>` and a
// query `List` compare equal.
func typeBase(t string) string {
	if i := strings.IndexByte(t, '<'); i >= 0 {
		t = t[:i]
	}
	return strings.ToLower(strings.TrimSpace(t))
}

// sigScore matches a function against a signature query: every wanted input type
// must map to a distinct input port, and (if given) the output type must match.
func sigScore(f *funk.Fn, wantIn []string, wantOut string) int {
	if wantOut != "" {
		ob := ""
		if len(f.Out) == 1 {
			ob = typeBase(f.Out[0].Type)
		}
		if ob != typeBase(wantOut) {
			return 0
		}
	}
	fnIn := make([]string, len(f.In))
	for i, p := range f.In {
		fnIn[i] = typeBase(p.Type)
	}
	used := make([]bool, len(fnIn))
	for _, w := range wantIn {
		wb, found := typeBase(w), false
		for i, ib := range fnIn {
			if !used[i] && ib == wb {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return 0
		}
	}
	score := 10
	if len(wantIn) == len(fnIn) {
		score += 5 // exact arity is a stronger match
	}
	return score
}

// searchStop are words too common to be signal in a task query.
var searchStop = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "in": true, "on": true,
	"is": true, "for": true, "and": true, "or": true, "with": true, "by": true, "from": true,
	"at": true, "be": true, "it": true, "that": true, "this": true, "into": true, "each": true,
	"compute": true, "return": true, "returns": true, "get": true, "give": true, "make": true,
	"create": true, "value": true, "values": true, "given": true, "using": true,
}

// textScore scores a function against query tokens, weighting the name/display and
// ignoring stopwords / very short tokens (so a full-sentence task query ranks on
// its meaningful words, not "the"/"of").
func textScore(f *funk.Fn, toks []string) int {
	name := strings.ToLower(f.Name + " " + f.Display)
	full := name + " " + strings.ToLower(f.Doc+" "+f.Examples)
	score := 0
	for _, t := range toks {
		if len(t) < 3 || searchStop[t] {
			continue
		}
		switch {
		case strings.Contains(name, t):
			score += 3
		case strings.Contains(full, t):
			score++
		}
	}
	return score
}
