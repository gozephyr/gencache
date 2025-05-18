package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestPrometheusMetricsExporterFunc(t *testing.T) {
	// Create a test registry
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	// Create exporter with unique name
	exporter := NewPrometheusMetricsExporter("test-cache-prom", map[string]string{
		"service": "test-service",
	})

	// Clean up after test
	defer func() {
		registry.Unregister(exporter.hits)
		registry.Unregister(exporter.misses)
		registry.Unregister(exporter.evictions)
		registry.Unregister(exporter.size)
		registry.Unregister(exporter.memory)
	}()

	t.Run("RecordHit", func(t *testing.T) {
		exporter.RecordHit()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.Hits)
	})

	t.Run("RecordMiss", func(t *testing.T) {
		exporter.RecordMiss()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.Misses)
	})

	t.Run("RecordEviction", func(t *testing.T) {
		exporter.RecordEviction()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.Evictions)
	})

	t.Run("UpdateSize", func(t *testing.T) {
		exporter.UpdateSize(100)
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(100), snapshot.Size)
	})

	t.Run("Reset", func(t *testing.T) {
		exporter.Reset()
		snapshot := exporter.GetSnapshot()
		assert.Equal(t, int64(0), snapshot.Hits)
		assert.Equal(t, int64(0), snapshot.Misses)
		assert.Equal(t, int64(0), snapshot.Evictions)
		assert.Equal(t, int64(0), snapshot.Size)
	})
}
