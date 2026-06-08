// Package search defines the pluggable tracker-search seam. v1 ships the
// interface + registry only; concrete site adapters land later.
package search

import "context"

type Result struct {
	Title    string `json:"title"`
	Magnet   string `json:"magnet"`
	Link     string `json:"link"`
	Size     int64  `json:"size"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Source   string `json:"source"`
}

type Adapter interface {
	Name() string
	Search(ctx context.Context, query string) ([]Result, error)
}

type Registry struct {
	adapters []Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	return &Registry{adapters: adapters}
}

func (r *Registry) Empty() bool { return len(r.adapters) == 0 }

// Search fans the query out to all adapters, ignoring individual failures.
func (r *Registry) Search(ctx context.Context, query string) ([]Result, error) {
	var out []Result
	for _, a := range r.adapters {
		res, err := a.Search(ctx, query)
		if err != nil {
			continue
		}
		out = append(out, res...)
	}
	return out, nil
}
