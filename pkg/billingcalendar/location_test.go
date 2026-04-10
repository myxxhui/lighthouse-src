package billingcalendar

import (
	"testing"
	"time"
)

func TestLocation_neverNil(t *testing.T) {
	t.Parallel()
	loc := Location()
	if loc == nil {
		t.Fatal("Location() must not be nil")
	}
	now := time.Now().UTC()
	_ = now.In(loc)
}
