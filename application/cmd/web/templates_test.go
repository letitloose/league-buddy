package main

import (
	"testing"
	"time"
)

// humanDateTime converts a UTC instant to US Eastern, so it must switch
// between EDT and EST across the daylight-saving boundary rather than
// applying a fixed offset.
func TestHumanDateTimeConvertsToEastern(t *testing.T) {
	summer := time.Date(2024, 7, 4, 12, 0, 0, 0, time.UTC)
	if got, want := humanDateTime(summer), "07/04/2024 8:00 AM EDT"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}

	winter := time.Date(2024, 1, 4, 12, 0, 0, 0, time.UTC)
	if got, want := humanDateTime(winter), "01/04/2024 7:00 AM EST"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
