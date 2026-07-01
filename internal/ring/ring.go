// Package ring defines the ordered deployment rings that every application is
// promoted through. Rings are shared across all applications.
//
// ADDING A RING is a single-place change: append an entry to the `ordered`
// slice below (in the correct position). Everything else — promotion order,
// API responses, and the UI — is derived from this list.
package ring

// Ring is one stage in the shared promotion pipeline.
type Ring struct {
	// Name is the stable identifier used in the API, config and storage.
	Name string `json:"name"`
	// Label is a human-friendly description shown in the UI.
	Label string `json:"label"`
}

// ordered is the single source of truth for the ring pipeline, lowest
// environment first. Promotion always moves from index i to index i+1.
var ordered = []Ring{
	{Name: "ring0", Label: "Dev"},
	{Name: "ring1", Label: "Integration"},
	{Name: "ring2", Label: "Acceptance"},
	{Name: "ring3", Label: "Production"},
}

// All returns a copy of the ordered ring list.
func All() []Ring {
	out := make([]Ring, len(ordered))
	copy(out, ordered)
	return out
}

// Names returns the ordered ring names.
func Names() []string {
	out := make([]string, len(ordered))
	for i, r := range ordered {
		out[i] = r.Name
	}
	return out
}

// Index returns the position of the named ring, or -1 if it does not exist.
func Index(name string) int {
	for i, r := range ordered {
		if r.Name == name {
			return i
		}
	}
	return -1
}

// Get returns the ring with the given name.
func Get(name string) (Ring, bool) {
	if i := Index(name); i >= 0 {
		return ordered[i], true
	}
	return Ring{}, false
}

// IsValid reports whether name refers to a known ring.
func IsValid(name string) bool {
	return Index(name) >= 0
}

// Next returns the ring immediately after name in the pipeline. ok is false
// when name is the last ring (production) or is unknown.
func Next(name string) (Ring, bool) {
	i := Index(name)
	if i < 0 || i+1 >= len(ordered) {
		return Ring{}, false
	}
	return ordered[i+1], true
}
