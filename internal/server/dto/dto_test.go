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

func TestNamespaceCostSummary(t *testing.T) {
	s := NamespaceCostSummary{Name: "default", Cost: 500, Grade: "Healthy", PodCount: 10}
	if s.Name != "default" || s.Cost != 500 {
		t.Errorf("NamespaceCostSummary: want name=default cost=500, got %s %v", s.Name, s.Cost)
	}
}
