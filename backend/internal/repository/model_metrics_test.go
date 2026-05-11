package repository

import (
	"math"
	"testing"
)

func TestComputeModelMetricsPerfectRanking(t *testing.T) {
	items := []labeledScore{
		{CaseID: 1, Label: "rumour", Score: 0.95},
		{CaseID: 2, Label: "rumor", Score: 0.90},
		{CaseID: 3, Label: "non_rumour", Score: 0.20},
		{CaseID: 4, Label: "non_rumour", Score: 0.10},
	}
	positiveSet := map[string]struct{}{
		"rumour": {},
		"rumor":  {},
	}

	metrics := computeModelMetrics(items, positiveSet)

	assertFloatPtrNear(t, "precision_at_10", metrics.PrecisionAt10, 0.5)
	assertFloatPtrNear(t, "precision_at_20", metrics.PrecisionAt20, 0.5)
	assertFloatPtrNear(t, "precision", metrics.Precision, 1.0)
	assertFloatPtrNear(t, "recall", metrics.Recall, 1.0)
	assertFloatPtrNear(t, "f1", metrics.F1, 1.0)
	assertFloatPtrNear(t, "roc_auc", metrics.ROCAUC, 1.0)
	assertFloatPtrNear(t, "pr_auc", metrics.PRAUC, 1.0)
	if metrics.BestThreshold == nil || *metrics.BestThreshold < 0.21 || *metrics.BestThreshold > 0.95 {
		t.Fatalf("unexpected best threshold: %v", metrics.BestThreshold)
	}
}

func TestPrecisionAtKMetricUsesAvailableRowsWhenKIsLarge(t *testing.T) {
	items := []labeledScore{
		{CaseID: 1, Label: "rumour", Score: 0.9},
		{CaseID: 2, Label: "non_rumour", Score: 0.8},
		{CaseID: 3, Label: "rumour", Score: 0.7},
	}
	positiveSet := map[string]struct{}{"rumour": {}}

	value := precisionAtKMetric(items, positiveSet, 10)

	assertFloatPtrNear(t, "precision_at_large_k", value, 2.0/3.0)
}

func TestROCAUCMetricReturnsNilWhenOnlyOneClass(t *testing.T) {
	items := []labeledScore{
		{CaseID: 1, Label: "rumour", Score: 0.9},
		{CaseID: 2, Label: "rumour", Score: 0.8},
	}
	positiveSet := map[string]struct{}{"rumour": {}}

	if value := rocAUCMetric(items, positiveSet); value != nil {
		t.Fatalf("expected nil roc_auc for one-class labels, got %v", *value)
	}
}

func assertFloatPtrNear(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %.6f", name, want)
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("%s = %.12f, want %.12f", name, *got, want)
	}
}
