package main

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"sync"
	"text/tabwriter"
	"time"
)

// tally counts the requests of one step, gateway and kind.
type tally struct {
	sent, ok, failed, lost int
	latencies              []time.Duration // of the successful answers
}

type key struct {
	step, gateway int
	kind          kind
}

// recorder collects the outcomes of the requests of the steps; those of the
// warm-up do not count.
type recorder struct {
	mu      sync.Mutex
	tallies map[key]*tally
	errors  map[string]int // the texts of the error answers
	strays  int            // answers with no request to belong to
}

func newRecorder() *recorder {
	return &recorder{tallies: make(map[key]*tally), errors: make(map[string]int)}
}

// tally returns the tally of a step, gateway and kind; the caller holds mu.
func (r *recorder) tally(step, g int, k kind) *tally {
	t := r.tallies[key{step, g, k}]
	if t == nil {
		t = &tally{}
		r.tallies[key{step, g, k}] = t
	}
	return t
}

func (r *recorder) sent(step, g int, k kind) {
	if step < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tally(step, g, k).sent++
}

// answer records an answer that came latency after its request was due;
// failure is the error it carries, if any.
func (r *recorder) answer(step, g int, k kind, latency time.Duration, failure string) {
	if step < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tally(step, g, k)
	if failure != "" {
		t.failed++
		r.errors[failure]++
		return
	}
	t.ok++
	t.latencies = append(t.latencies, latency)
}

func (r *recorder) lost(step, g int, k kind) {
	if step < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tally(step, g, k).lost++
}

func (r *recorder) stray() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strays++
}

func (r *recorder) report(c config, start time.Time) *report {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &report{c: c, start: start, tallies: r.tallies, errors: r.errors, strays: r.strays}
}

// report is the outcome of a run.
type report struct {
	c       config
	start   time.Time // of the warm-up
	tallies map[key]*tally
	errors  map[string]int
	strays  int
}

// print writes a row per step, gateway and kind of request, and one for all
// the kinds together.
func (r *report) print(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "step\trps\tfrom (UTC)\tgateway\trequest\tsent\tok\terrors\tlost\tok/s\tp50 ms\tp99 ms")
	for i, rate := range r.c.rates {
		from := r.start.Add(r.c.warmup + time.Duration(i)*r.c.step).UTC().Format(time.TimeOnly)
		for g, gw := range r.c.gateways {
			var all tally
			row := func(name string, t *tally) {
				sorted := slices.Sorted(slices.Values(t.latencies))
				_, _ = fmt.Fprintf(tw, "%d\t%g\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%.1f\t%s\t%s\n",
					i+1, rate, from, gw.name, name, t.sent, t.ok, t.failed, t.lost,
					float64(t.ok)/r.c.step.Seconds(), quantile(sorted, 0.5), quantile(sorted, 0.99))
			}
			for k := range kinds {
				t := r.tallies[key{i, g, k}]
				if t == nil {
					continue
				}
				row(kindNames[k], t)
				all.sent, all.ok, all.failed, all.lost = all.sent+t.sent, all.ok+t.ok, all.failed+t.failed, all.lost+t.lost
				all.latencies = append(all.latencies, t.latencies...)
			}
			row("all", &all)
		}
	}
	_ = tw.Flush()

	if len(r.errors) > 0 {
		_, _ = fmt.Fprintln(w, "\nerror answers:")
		texts := slices.SortedFunc(maps.Keys(r.errors), func(a, b string) int {
			return cmp.Or(cmp.Compare(r.errors[b], r.errors[a]), cmp.Compare(a, b))
		})
		for _, text := range texts {
			_, _ = fmt.Fprintf(w, "%8d  %s\n", r.errors[text], text)
		}
	}
	if r.strays > 0 {
		_, _ = fmt.Fprintf(w, "\n%d answers matched no request: the latencies may be off\n", r.strays)
	}
}

// quantile returns the q-quantile of sorted latencies in milliseconds, by
// the nearest rank, or "-" if there are none.
func quantile(sorted []time.Duration, q float64) string {
	if len(sorted) == 0 {
		return "-"
	}
	rank := max(int(math.Ceil(q*float64(len(sorted))))-1, 0)
	return fmt.Sprintf("%.2f", float64(sorted[rank])/float64(time.Millisecond))
}
