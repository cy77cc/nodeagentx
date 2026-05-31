package collector

// Processor is a plugin that transforms metrics in-flight.
type Processor interface {
	Init(cfg map[string]any) error
	Apply(in []*Metric) []*Metric
	SampleConfig() string
}
