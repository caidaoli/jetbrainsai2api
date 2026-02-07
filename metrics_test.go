package main

import "testing"

func TestNewAtomicRequestStatsHistorySize(t *testing.T) {
	stats := NewAtomicRequestStats(3)
	defer stats.Stop()

	batch := []RequestRecord{
		{Model: "m1"},
		{Model: "m2"},
		{Model: "m3"},
		{Model: "m4"},
	}
	stats.flushHistoryBatch(batch)

	history := stats.GetHistory()
	if len(history) != 3 {
		t.Fatalf("history size mismatch: got %d, want 3", len(history))
	}
	if history[0].Model != "m2" || history[2].Model != "m4" {
		t.Fatalf("history order mismatch: got %#v", history)
	}
}

func TestNewAtomicRequestStatsHistorySizeFallback(t *testing.T) {
	stats := NewAtomicRequestStats(0)
	defer stats.Stop()

	if stats.historyLimit != HistoryBufferSize {
		t.Fatalf("historyLimit mismatch: got %d, want %d", stats.historyLimit, HistoryBufferSize)
	}
}
