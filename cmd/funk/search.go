package main

import (
	"encoding/json"
	"fmt"
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
	limits, rest := takeFlag(rest, "--limit")
	limit := 20
	if len(limits) > 0 {
		if n, err := strconv.Atoi(limits[len(limits)-1]); err == nil {
			limit = n
		}
	}
	if len(rest) == 0 {
		return fmt.Errorf("search: usage: funk search [--json] [--limit N] <query | in… -> out>")
	}
	query := strings.Join(rest, " ")
	lib, err := loadLibrary(files)
	if err != nil {
		return err
	}

	rows := searchResults(lib, query, limit)
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
	for _, h := range hits {
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

// textScore scores a function against query tokens, weighting the name/display.
func textScore(f *funk.Fn, toks []string) int {
	name := strings.ToLower(f.Name + " " + f.Display)
	full := strings.ToLower(f.Name + " " + f.Display + " " + f.Doc + " " + f.Examples)
	score := 0
	for _, t := range toks {
		switch {
		case strings.Contains(name, t):
			score += 3
		case strings.Contains(full, t):
			score++
		}
	}
	return score
}
