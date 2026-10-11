// Package budget carries the ZIP recursion counters in a context.
// Callers outside this module cannot see the key.
package budget

import "context"

// State is shared by a ZIP and the members it sends back to the shell.
type State struct {
	Depth int // how many zip layers are already open, not counting the one about to run
	Files *int
	Left  *int64
}

type key struct{}

// With stores st. A nil st returns ctx unchanged.
func With(ctx context.Context, st *State) context.Context {
	if st == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, st)
}

// Get returns the state stored by With, or nil.
func Get(ctx context.Context) *State {
	st, _ := ctx.Value(key{}).(*State)
	return st
}
