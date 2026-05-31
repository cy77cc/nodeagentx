package collector

import (
	"sync"
	"time"
)

// Accumulator is the interface inputs use to emit metrics.
type Accumulator interface {
	AddFields(name string, tags map[string]string, fields map[string]any)
	AddGauge(name string, tags map[string]string, fields map[string]any)
	AddCounter(name string, tags map[string]string, fields map[string]any)
	AddFieldsWithTimestamp(name string, tags map[string]string, fields map[string]any, ts time.Time)
	AddGaugeWithTimestamp(name string, tags map[string]string, fields map[string]any, ts time.Time)
	AddCounterWithTimestamp(name string, tags map[string]string, fields map[string]any, ts time.Time)
	Collect() []*Metric
}

type accumulator struct {
	mu      sync.Mutex
	metrics []*Metric
	maxSize int
}

// NewAccumulator creates an Accumulator with the given max buffer size.
// When full, new metrics are dropped (DropNewest policy).
func NewAccumulator(maxSize int) Accumulator {
	return &accumulator{
		metrics: make([]*Metric, 0, maxSize),
		maxSize: maxSize,
	}
}

func (a *accumulator) AddFields(name string, tags map[string]string, fields map[string]any) {
	a.add(name, tags, fields, Gauge, time.Now())
}

func (a *accumulator) AddGauge(name string, tags map[string]string, fields map[string]any) {
	a.add(name, tags, fields, Gauge, time.Now())
}

func (a *accumulator) AddCounter(name string, tags map[string]string, fields map[string]any) {
	a.add(name, tags, fields, Counter, time.Now())
}

func (a *accumulator) AddFieldsWithTimestamp(name string, tags map[string]string, fields map[string]any, ts time.Time) {
	a.add(name, tags, fields, Gauge, ts)
}

func (a *accumulator) AddGaugeWithTimestamp(name string, tags map[string]string, fields map[string]any, ts time.Time) {
	a.add(name, tags, fields, Gauge, ts)
}

func (a *accumulator) AddCounterWithTimestamp(name string, tags map[string]string, fields map[string]any, ts time.Time) {
	a.add(name, tags, fields, Counter, ts)
}

func (a *accumulator) add(name string, tags map[string]string, fields map[string]any, mt MetricType, ts time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.metrics) >= a.maxSize {
		return // DropNewest
	}
	a.metrics = append(a.metrics, NewMetric(name, tags, fields, mt, ts))
}

// Collect returns accumulated metrics and resets the buffer.
func (a *accumulator) Collect() []*Metric {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := a.metrics
	a.metrics = make([]*Metric, 0, a.maxSize)
	return result
}
