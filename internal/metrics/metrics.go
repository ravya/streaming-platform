// Package metrics is a tiny Prometheus text-format registry (counters,
// histograms, gauges). In a larger codebase swap for prometheus/client_golang;
// the exposition format (and so dashboards/alerts) stays identical.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

var defBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5}

type hist struct {
	counts []uint64
	sum    float64
	n      uint64
}

type Registry struct {
	mu       sync.Mutex
	counters map[string]float64
	hists    map[string]*hist
	gauges   map[string]func() float64
}

func NewRegistry() *Registry {
	return &Registry{counters: map[string]float64{}, hists: map[string]*hist{}, gauges: map[string]func() float64{}}
}

func key(name string, labels []string) string {
	if len(labels) == 0 {
		return name
	}
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, fmt.Sprintf(`%s=%q`, labels[i], labels[i+1]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

// Inc increments a counter. labels are alternating key, value.
func (r *Registry) Inc(name string, labels ...string) {
	r.mu.Lock()
	r.counters[key(name, labels)]++
	r.mu.Unlock()
}

func (r *Registry) Observe(name string, v float64, labels ...string) {
	k := key(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.hists[k]
	if h == nil {
		h = &hist{counts: make([]uint64, len(defBuckets))}
		r.hists[k] = h
	}
	for i, b := range defBuckets {
		if v <= b {
			h.counts[i]++
		}
	}
	h.sum += v
	h.n++
}

func (r *Registry) Gauge(name string, fn func() float64) {
	r.mu.Lock()
	r.gauges[name] = fn
	r.mu.Unlock()
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		r.write(w)
	})
}

func (r *Registry) write(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.counters))
	for k := range r.counters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s %g\n", k, r.counters[k])
	}
	hk := make([]string, 0, len(r.hists))
	for k := range r.hists {
		hk = append(hk, k)
	}
	sort.Strings(hk)
	for _, k := range hk {
		h := r.hists[k]
		name, lbl := k, ""
		if i := strings.IndexByte(k, '{'); i >= 0 {
			name, lbl = k[:i], strings.TrimSuffix(k[i+1:], "}")+","
		}
		for i, b := range defBuckets {
			fmt.Fprintf(w, "%s_bucket{%sle=\"%g\"} %d\n", name, lbl, b, h.counts[i])
		}
		fmt.Fprintf(w, "%s_bucket{%sle=\"+Inf\"} %d\n", name, lbl, h.n)
		fmt.Fprintf(w, "%s_sum{%s} %g\n%s_count{%s} %d\n", name, strings.TrimSuffix(lbl, ","), h.sum, name, strings.TrimSuffix(lbl, ","), h.n)
	}
	gk := make([]string, 0, len(r.gauges))
	for k := range r.gauges {
		gk = append(gk, k)
	}
	sort.Strings(gk)
	for _, k := range gk {
		fmt.Fprintf(w, "%s %g\n", k, r.gauges[k]())
	}
}
