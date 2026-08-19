package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metric names conforming strictly to design.md Section 4 conventions.
const (
	MetricRTTSeconds           = "internet_monitor_rtt_seconds"
	MetricJitterSeconds        = "internet_monitor_jitter_seconds"
	MetricPacketLossRatio      = "internet_monitor_packet_loss_ratio"
	MetricSpeedtestBytesPerSec = "internet_monitor_speedtest_bytes_per_second"
)

// Standard label / attribute keys.
const (
	AttrTarget    = attribute.Key("target")
	AttrProbe     = attribute.Key("probe")
	AttrIPVersion = attribute.Key("ip_version")
	AttrDirection = attribute.Key("direction")
)

// Metrics holds the instantiated OpenTelemetry instruments.
type Metrics struct {
	rttHistogram      metric.Float64Histogram
	jitterHistogram   metric.Float64Histogram
	packetLossGauge   metric.Float64Gauge
	speedtestGauge    metric.Float64Gauge
}

// NewMetrics instantiates and registers all standard metrics instruments.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	if meter == nil {
		return nil, fmt.Errorf("cannot create metrics with nil meter")
	}

	rttHist, err := meter.Float64Histogram(
		MetricRTTSeconds,
		metric.WithDescription("Round-trip time latency measurement in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s histogram: %w", MetricRTTSeconds, err)
	}

	jitterHist, err := meter.Float64Histogram(
		MetricJitterSeconds,
		metric.WithDescription("Latency variation (jitter) between consecutive measurements in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s histogram: %w", MetricJitterSeconds, err)
	}

	packetLoss, err := meter.Float64Gauge(
		MetricPacketLossRatio,
		metric.WithDescription("Packet loss ratio between 0.0 (no loss) and 1.0 (100% loss)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s gauge: %w", MetricPacketLossRatio, err)
	}

	speedtest, err := meter.Float64Gauge(
		MetricSpeedtestBytesPerSec,
		metric.WithDescription("Speedtest measured network bandwidth in bytes per second"),
		metric.WithUnit("By/s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s gauge: %w", MetricSpeedtestBytesPerSec, err)
	}

	return &Metrics{
		rttHistogram:    rttHist,
		jitterHistogram: jitterHist,
		packetLossGauge: packetLoss,
		speedtestGauge:  speedtest,
	}, nil
}

// RecordRTT records a round-trip time latency measurement.
func (m *Metrics) RecordRTT(ctx context.Context, rttSeconds float64, target, probe, ipVersion string, extra ...attribute.KeyValue) {
	attrs := []attribute.KeyValue{
		AttrTarget.String(target),
		AttrProbe.String(probe),
	}
	if ipVersion != "" {
		attrs = append(attrs, AttrIPVersion.String(ipVersion))
	}
	attrs = append(attrs, extra...)

	m.rttHistogram.Record(ctx, rttSeconds, metric.WithAttributes(attrs...))
}

// RecordJitter records a latency jitter measurement.
func (m *Metrics) RecordJitter(ctx context.Context, jitterSeconds float64, target, probe, ipVersion string, extra ...attribute.KeyValue) {
	attrs := []attribute.KeyValue{
		AttrTarget.String(target),
		AttrProbe.String(probe),
	}
	if ipVersion != "" {
		attrs = append(attrs, AttrIPVersion.String(ipVersion))
	}
	attrs = append(attrs, extra...)

	m.jitterHistogram.Record(ctx, jitterSeconds, metric.WithAttributes(attrs...))
}

// RecordPacketLoss records the packet loss ratio (0.0 to 1.0).
func (m *Metrics) RecordPacketLoss(ctx context.Context, lossRatio float64, target, probe, ipVersion string, extra ...attribute.KeyValue) {
	attrs := []attribute.KeyValue{
		AttrTarget.String(target),
		AttrProbe.String(probe),
	}
	if ipVersion != "" {
		attrs = append(attrs, AttrIPVersion.String(ipVersion))
	}
	attrs = append(attrs, extra...)

	m.packetLossGauge.Record(ctx, lossRatio, metric.WithAttributes(attrs...))
}

// RecordSpeedtest records bandwidth throughput in bytes/second.
func (m *Metrics) RecordSpeedtest(ctx context.Context, bytesPerSec float64, target, direction string, extra ...attribute.KeyValue) {
	attrs := []attribute.KeyValue{
		AttrTarget.String(target),
		AttrProbe.String("speedtest"),
		AttrDirection.String(direction),
	}
	attrs = append(attrs, extra...)

	m.speedtestGauge.Record(ctx, bytesPerSec, metric.WithAttributes(attrs...))
}
