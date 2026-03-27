package dto

import (
	"testing"
	"time"
)

func TestGlobalCostResponse(t *testing.T) {
	r := GlobalCostResponse{
		TotalCost:        1000,
		TotalOptimizable: 200,
		GlobalEfficiency: 80,
		Timestamp:        time.Now().UTC(),
	}
	if r.TotalCost != 1000 {
		t.Errorf("TotalCost want 1000, got %v", r.TotalCost)
	}
}

func TestApplyFinOpsGlobalMetadata(t *testing.T) {
	r := &GlobalCostResponse{TotalCost: 1, Timestamp: time.Now().UTC()}
	ApplyFinOpsGlobalMetadata(r, "")
	if r.Metadata != nil {
		t.Fatal("want no metadata when track empty")
	}
	ApplyFinOpsGlobalMetadata(r, "technical")
	if r.Metadata == nil || r.Metadata.EffectiveTrack != "technical" {
		t.Fatalf("EffectiveTrack want technical, got %+v", r.Metadata)
	}
}

func TestNamespaceCostSummary(t *testing.T) {
	s := NamespaceCostSummary{Name: "default", Cost: 500, Grade: "Healthy", PodCount: 10}
	if s.Name != "default" || s.Cost != 500 {
		t.Errorf("NamespaceCostSummary: want name=default cost=500, got %s %v", s.Name, s.Cost)
	}
}
