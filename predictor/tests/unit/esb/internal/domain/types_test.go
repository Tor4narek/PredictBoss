package domain

import "testing"

func TestCallStatusTerminal(t *testing.T) {
	for _, test := range []struct {
		status CallStatus
		want   bool
	}{
		{status: CallQueued},
		{status: CallSubmitting},
		{status: CallStarted},
		{status: CallRunning},
		{status: CallProcessing},
		{status: CallCompleted, want: true},
		{status: CallFailed, want: true},
		{status: CallCancelled, want: true},
		{status: CallInterrupted, want: true},
		{status: "unknown"},
	} {
		if got := test.status.Terminal(); got != test.want {
			t.Errorf("Terminal(%q) = %t, want %t", test.status, got, test.want)
		}
	}
}

func TestMetricTypeValid(t *testing.T) {
	for _, metricType := range []MetricType{
		MetricNumeric, MetricCurrency, MetricPercentage, MetricCount,
		MetricRatio, MetricScore, MetricText,
	} {
		if !metricType.Valid() {
			t.Errorf("Valid(%q) = false, want true", metricType)
		}
	}
	if (MetricType("unknown")).Valid() {
		t.Fatal("Valid(unknown) = true, want false")
	}
}
