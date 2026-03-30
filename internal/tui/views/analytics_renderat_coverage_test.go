package views

import (
	"testing"
)

func TestAnalyticsView_RenderAt(t *testing.T) {
	v := NewAnalyticsView()
	v.SetData(sampleAnalyticsData())
	// RenderAt should set dimensions and return non-empty content.
	got := v.RenderAt(80, 24)
	if got == "" {
		t.Error("RenderAt should return non-empty content")
	}
}

func TestAnalyticsView_RenderAt_SmallDimensions(t *testing.T) {
	v := NewAnalyticsView()
	// Just verify no panic with very small dimensions.
	_ = v.RenderAt(10, 5)
}
