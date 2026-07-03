package funk

// TraceEvent is one step in a run's execution path.
type TraceEvent struct {
	Kind   string      `json:"kind"` // "call" | "branch" | "terminal" | "enter"
	Fn     string      `json:"fn,omitempty"`
	Node   string      `json:"node,omitempty"` // stable graph-node id = the call site's Pos "line:col" (matches `funk graph --json`)
	Detail string      `json:"detail,omitempty"`
	Value  interface{} `json:"value,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// RunReport is the structured trace of a run — the run as data, which the agent
// reads about itself (docs/01 property #4: self-observable over structure).
type RunReport struct {
	Ref    string       `json:"ref"`
	OK     bool         `json:"ok"`
	Value  interface{}  `json:"value,omitempty"`
	Error  string       `json:"error,omitempty"`
	Events []TraceEvent `json:"events"`
}
