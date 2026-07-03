package funk

// TraceEvent is one step in a run's execution path.
type TraceEvent struct {
	Kind   string      `json:"kind"` // "call" | "branch" | "terminal"
	Fn     string      `json:"fn,omitempty"`
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
