package app

import "github.com/prometheus/client_golang/prometheus"

// MetricsRegistry holds all agent Prometheus metrics on an isolated registry.
type MetricsRegistry struct {
	registry *prometheus.Registry

	Uptime           prometheus.Gauge
	GRPCConnected    prometheus.Gauge
	TasksRunning     prometheus.Gauge
	TasksCompleted   prometheus.Counter
	TasksFailed      *prometheus.CounterVec
	MetricsCollected prometheus.Counter
	PipelineErrors   *prometheus.CounterVec
	PluginRequests   *prometheus.CounterVec
	GRPCReconnects   prometheus.Counter

	CPUUsage    prometheus.Gauge
	MemoryUsage prometheus.Gauge
	DiskUsage   prometheus.Gauge
	Load1       prometheus.Gauge
	Load5       prometheus.Gauge
	Load15      prometheus.Gauge
	NetSent     prometheus.Counter
	NetRecv     prometheus.Counter

	GatewayTunnelsActive prometheus.Gauge
	GatewayTunnelBytes   prometheus.Counter
	GatewayTunnelErrors  prometheus.Counter
	GatewayProxyRequests prometheus.Counter
	GatewayProxyLatency  prometheus.Histogram
}

// NewMetricsRegistry creates a new isolated Prometheus registry with all agent metrics.
func NewMetricsRegistry() *MetricsRegistry {
	reg := prometheus.NewRegistry()
	m := &MetricsRegistry{
		registry: reg,
		Uptime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_uptime_seconds", Help: "Agent uptime in seconds",
		}),
		GRPCConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_grpc_connected", Help: "Whether gRPC connection is active (1=connected, 0=disconnected)",
		}),
		TasksRunning: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_tasks_running", Help: "Number of currently running tasks",
		}),
		TasksCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_tasks_completed_total", Help: "Total completed tasks",
		}),
		TasksFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "opsagent_tasks_failed_total", Help: "Total failed tasks",
		}, []string{"task_type", "error_code"}),
		MetricsCollected: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_metrics_collected_total", Help: "Total metrics collected by pipeline",
		}),
		PipelineErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "opsagent_pipeline_errors_total", Help: "Total pipeline processing errors",
		}, []string{"stage", "plugin"}),
		PluginRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "opsagent_plugin_requests_total", Help: "Total plugin runtime requests",
		}, []string{"plugin", "task_type", "status"}),
		GRPCReconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_grpc_reconnects_total", Help: "Total gRPC reconnection attempts",
		}),
		CPUUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_cpu_usage_percent", Help: "CPU usage percent",
		}),
		MemoryUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_memory_usage_percent", Help: "Memory usage percent",
		}),
		DiskUsage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_disk_usage_percent", Help: "Disk usage percent",
		}),
		Load1: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_load1", Help: "Host load average over 1 minute",
		}),
		Load5: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_load5", Help: "Host load average over 5 minutes",
		}),
		Load15: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_load15", Help: "Host load average over 15 minutes",
		}),
		NetSent: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_network_bytes_sent_total", Help: "Total bytes sent",
		}),
		NetRecv: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_network_bytes_recv_total", Help: "Total bytes received",
		}),
		GatewayTunnelsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "opsagent_gateway_tunnels_active", Help: "Number of active gateway tunnels",
		}),
		GatewayTunnelBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_gateway_tunnel_bytes_total", Help: "Total bytes tunneled",
		}),
		GatewayTunnelErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_gateway_tunnel_errors_total", Help: "Total tunnel errors",
		}),
		GatewayProxyRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "opsagent_gateway_proxy_requests_total", Help: "Total proxy requests",
		}),
		GatewayProxyLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "opsagent_gateway_proxy_latency_seconds",
			Help:    "Proxy command execution latency",
			Buckets: prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(
		m.Uptime, m.GRPCConnected, m.TasksRunning,
		m.TasksCompleted, m.TasksFailed, m.MetricsCollected,
		m.PipelineErrors, m.PluginRequests, m.GRPCReconnects,
		m.CPUUsage, m.MemoryUsage, m.DiskUsage,
		m.Load1, m.Load5, m.Load15,
		m.NetSent, m.NetRecv,
		m.GatewayTunnelsActive, m.GatewayTunnelBytes, m.GatewayTunnelErrors,
		m.GatewayProxyRequests, m.GatewayProxyLatency,
	)

	// Seed CounterVec metrics with a default label combination so they
	// appear in Gather() output even before any real observations.
	m.TasksFailed.WithLabelValues("", "")
	m.PipelineErrors.WithLabelValues("", "")
	m.PluginRequests.WithLabelValues("", "", "")

	return m
}

// Registry returns the underlying prometheus.Registry.
func (m *MetricsRegistry) Registry() *prometheus.Registry { return m.registry }

// UpdateSystemMetrics sets the system gauge values.
func (m *MetricsRegistry) UpdateSystemMetrics(cpu, mem, disk, load1, load5, load15 float64) {
	m.CPUUsage.Set(cpu)
	m.MemoryUsage.Set(mem)
	m.DiskUsage.Set(disk)
	m.Load1.Set(load1)
	m.Load5.Set(load5)
	m.Load15.Set(load15)
}

// IncTasksCompleted increments the tasks completed counter.
func (m *MetricsRegistry) IncTasksCompleted() { m.TasksCompleted.Inc() }

// IncTasksFailed increments the tasks failed counter with labels.
func (m *MetricsRegistry) IncTasksFailed(t, e string) {
	m.TasksFailed.WithLabelValues(t, e).Inc()
}

// IncMetricsCollected increments the metrics collected counter.
func (m *MetricsRegistry) IncMetricsCollected() { m.MetricsCollected.Inc() }

// IncGRPCReconnects increments the gRPC reconnects counter.
func (m *MetricsRegistry) IncGRPCReconnects() { m.GRPCReconnects.Inc() }

// IncPipelineErrors increments the pipeline errors counter with labels.
func (m *MetricsRegistry) IncPipelineErrors(stage, plugin string) {
	m.PipelineErrors.WithLabelValues(stage, plugin).Inc()
}

// IncPluginRequests increments the plugin requests counter with labels.
func (m *MetricsRegistry) IncPluginRequests(plugin, taskType, status string) {
	m.PluginRequests.WithLabelValues(plugin, taskType, status).Inc()
}
