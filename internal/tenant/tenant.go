// Package tenant holds per-tenant configuration (quotas, hot-retention override...).
// In production this lives in a config service / DynamoDB and is cached with a short TTL.
package tenant

import (
	"encoding/json"
	"os"
)

type Config struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	RPS   float64 `json:"rps"`   // sustained requests/sec quota
	Burst int     `json:"burst"` // token-bucket burst
}

type Registry struct{ m map[string]Config }

func Load(path string) (*Registry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cs []Config
	if err := json.Unmarshal(b, &cs); err != nil {
		return nil, err
	}
	return NewRegistry(cs...), nil
}

func NewRegistry(cs ...Config) *Registry {
	r := &Registry{m: map[string]Config{}}
	for _, c := range cs {
		r.m[c.ID] = c
	}
	return r
}

func (r *Registry) Get(id string) (Config, bool) { c, ok := r.m[id]; return c, ok }
