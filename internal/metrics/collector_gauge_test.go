package metrics

import (
	"bytes"
	"testing"
)

func TestGaugeInc(t *testing.T) {
	g := NewCollector().Gauge("inc_test")
	g.Set(10.0)
	g.Inc()
	if g.Value() != 11.0 {
		t.Errorf("Gauge after Inc = %f, want 11.0", g.Value())
	}
}

func TestGaugeDec(t *testing.T) {
	g := NewCollector().Gauge("dec_test")
	g.Set(10.0)
	g.Dec()
	if g.Value() != 9.0 {
		t.Errorf("Gauge after Dec = %f, want 9.0", g.Value())
	}
}

func TestGaugeIncDec(t *testing.T) {
	g := NewCollector().Gauge("incdec_test")
	g.Set(0.0)
	g.Inc()
	g.Inc()
	g.Inc()
	g.Dec()
	if g.Value() != 2.0 {
		t.Errorf("Gauge after Incx3 Decx1 = %f, want 2.0", g.Value())
	}
}

func TestGaugeIncFromZero(t *testing.T) {
	g := NewCollector().Gauge("zero_inc_test")
	// Default value is 0
	g.Inc()
	if g.Value() != 1.0 {
		t.Errorf("Gauge after Inc from 0 = %f, want 1.0", g.Value())
	}
}

func TestGaugeDecNegative(t *testing.T) {
	g := NewCollector().Gauge("neg_dec_test")
	g.Dec()
	if g.Value() != -1.0 {
		t.Errorf("Gauge after Dec from 0 = %f, want -1.0", g.Value())
	}
}

func TestCollectorIncGauge(t *testing.T) {
	c := NewCollector()
	c.IncGauge("conn")
	c.IncGauge("conn")
	c.IncGauge("conn")

	g := c.Gauge("conn")
	if g.Value() != 3.0 {
		t.Errorf("IncGauge x3 = %f, want 3.0", g.Value())
	}
}

func TestCollectorDecGauge(t *testing.T) {
	c := NewCollector()
	c.SetGauge("conn", 5.0)
	c.DecGauge("conn")

	g := c.Gauge("conn")
	if g.Value() != 4.0 {
		t.Errorf("SetGauge(5) then DecGauge = %f, want 4.0", g.Value())
	}
}

func TestCollectorIncDecGaugeConcurrent(t *testing.T) {
	c := NewCollector()
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.IncGauge("concurrent")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	g := c.Gauge("concurrent")
	if g.Value() != 1000.0 {
		t.Errorf("Concurrent IncGauge = %f, want 1000.0", g.Value())
	}

	// Now decrement
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.DecGauge("concurrent")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	if g.Value() != 0.0 {
		t.Errorf("After DecGauge = %f, want 0.0", g.Value())
	}
}

func TestGaugePrometheusFormat(t *testing.T) {
	c := NewCollector()
	g := c.Gauge("active.conns")
	g.Set(42.0)
	g.Inc()

	c.IncGauge("another.metric")

	var buf bytes.Buffer
	c.PrometheusFormat(&buf)
	output := buf.String()

	if !contains(output, "dockrouter_active_conns 43") {
		t.Errorf("Expected gauge value 43 in output: %s", output)
	}
	if !contains(output, "dockrouter_another_metric 1") {
		t.Errorf("Expected gauge value 1 in output: %s", output)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
