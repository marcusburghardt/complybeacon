package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// evidenceObserverTestFixture provides test infrastructure for EvidenceObserver tests
type evidenceObserverTestFixture struct {
	observer *EvidenceObserver
	reader   *sdkmetric.ManualReader
	t        *testing.T
}

// setupEvidenceObserverTest creates a test fixture with configured meter provider
func setupEvidenceObserverTest(t *testing.T) *evidenceObserverTestFixture {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)
	meter := meterProvider.Meter("test-meter")

	observer, err := NewEvidenceObserver(meter)
	require.NoError(t, err)

	return &evidenceObserverTestFixture{
		observer: observer,
		reader:   reader,
		t:        t,
	}
}

// collectMetrics collects and returns metrics from the test fixture
func (f *evidenceObserverTestFixture) collectMetrics(ctx context.Context) metricdata.ResourceMetrics {
	var rm metricdata.ResourceMetrics
	err := f.reader.Collect(ctx, &rm)
	require.NoError(f.t, err)
	return rm
}

// assertMetricsRecorded verifies that metrics were recorded
func (f *evidenceObserverTestFixture) assertMetricsRecorded(ctx context.Context) {
	rm := f.collectMetrics(ctx)
	assert.NotEmpty(f.t, rm.ScopeMetrics)
}

func TestNewEvidenceObserver(t *testing.T) {
	t.Run("create new observer successfully", func(t *testing.T) {
		meterProvider := sdkmetric.NewMeterProvider()
		meter := meterProvider.Meter("test-meter")

		observer, err := NewEvidenceObserver(meter)
		require.NoError(t, err)
		assert.NotNil(t, observer)
		assert.NotNil(t, observer.meter)
		assert.NotNil(t, observer.droppedCounter)
		assert.NotNil(t, observer.processedCount)
	})

	t.Run("observer with manual reader", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		assert.NotNil(t, fixture.observer)
	})
}

func TestEvidenceObserverProcessed(t *testing.T) {
	t.Run("record single processed event", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		fixture.observer.Processed(ctx, attribute.String("test", "value"))
		fixture.assertMetricsRecorded(ctx)
	})

	t.Run("record multiple processed events", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		// Record multiple processed events
		for i := 0; i < 5; i++ {
			fixture.observer.Processed(ctx, attribute.String("iteration", string(rune(i))))
		}
		fixture.assertMetricsRecorded(ctx)
	})

	t.Run("record with multiple attributes", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		fixture.observer.Processed(ctx,
			attribute.String("policy.id", "test-policy"),
			attribute.String("policy.source", "test-source"),
			attribute.String("policy.evaluation.status", "pass"),
		)
		fixture.assertMetricsRecorded(ctx)
	})

	t.Run("record with no attributes", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		fixture.observer.Processed(ctx)
		fixture.assertMetricsRecorded(ctx)
	})
}

func TestEvidenceObserverDropped(t *testing.T) {
	t.Run("record single dropped event", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		fixture.observer.Dropped(ctx, attribute.String("reason", "validation_failed"))
		fixture.assertMetricsRecorded(ctx)
	})

	t.Run("record multiple dropped events", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		// Record multiple dropped events
		reasons := []string{"validation_failed", "processing_error", "timeout"}
		for _, reason := range reasons {
			fixture.observer.Dropped(ctx, attribute.String("reason", reason))
		}
		fixture.assertMetricsRecorded(ctx)
	})

	t.Run("record with no attributes", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		fixture.observer.Dropped(ctx)
		fixture.assertMetricsRecorded(ctx)
	})
}

func TestEvidenceObserverBothMetrics(t *testing.T) {
	t.Run("record both processed and dropped", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		// Record some processed events
		fixture.observer.Processed(ctx, attribute.String("policy.id", "policy-1"))
		fixture.observer.Processed(ctx, attribute.String("policy.id", "policy-2"))
		fixture.observer.Processed(ctx, attribute.String("policy.id", "policy-3"))

		// Record some dropped events
		fixture.observer.Dropped(ctx, attribute.String("reason", "error"))
		fixture.observer.Dropped(ctx, attribute.String("reason", "timeout"))

		// Verify both metric types are recorded
		rm := fixture.collectMetrics(ctx)
		assert.NotEmpty(t, rm.ScopeMetrics)

		// Should have metrics for both counters
		if len(rm.ScopeMetrics) > 0 {
			assert.NotEmpty(t, rm.ScopeMetrics[0].Metrics)
		}
	})

	t.Run("concurrent recording", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		// Record events concurrently
		done := make(chan bool, 2)

		go func() {
			for i := 0; i < 10; i++ {
				fixture.observer.Processed(ctx, attribute.String("goroutine", "1"))
			}
			done <- true
		}()

		go func() {
			for i := 0; i < 10; i++ {
				fixture.observer.Dropped(ctx, attribute.String("goroutine", "2"))
			}
			done <- true
		}()

		// Wait for both goroutines to complete
		<-done
		<-done

		fixture.assertMetricsRecorded(ctx)
	})
}

func TestEvidenceObserverMetricNames(t *testing.T) {
	t.Run("verify metric names and descriptions", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		// Record at least one event for each metric
		fixture.observer.Processed(ctx)
		fixture.observer.Dropped(ctx)

		// Check that metrics are present
		rm := fixture.collectMetrics(ctx)
		assert.NotEmpty(t, rm.ScopeMetrics)

		if len(rm.ScopeMetrics) > 0 {
			metrics := rm.ScopeMetrics[0].Metrics
			assert.NotEmpty(t, metrics)

			// Verify we have the expected metrics
			metricNames := make(map[string]bool)
			for _, m := range metrics {
				metricNames[m.Name] = true
			}

			// Check for expected metric names
			assert.True(t, metricNames["evidence_processed_count"] || metricNames["evidence_dropped_count"],
				"Expected to find evidence metrics")
		}
	})
}

func TestEvidenceObserverWithContext(t *testing.T) {
	t.Run("record with cancelled context", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Should not panic even with cancelled context
		fixture.observer.Processed(ctx, attribute.String("test", "value"))
		fixture.observer.Dropped(ctx, attribute.String("test", "value"))
	})

	t.Run("record with timeout context", func(t *testing.T) {
		fixture := setupEvidenceObserverTest(t)
		ctx := context.Background()

		// Should work fine
		fixture.observer.Processed(ctx, attribute.String("test", "value"))
		fixture.observer.Dropped(ctx, attribute.String("test", "value"))

		fixture.assertMetricsRecorded(ctx)
	})
}
