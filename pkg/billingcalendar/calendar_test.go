package billingcalendar

import (
	"testing"
	"time"
)

func TestPreviousMonthYYYYMM_March31Shanghai(t *testing.T) {
	// 与生产一致使用 billingcalendar.Location()（无 zoneinfo 时亦可运行）
	loc := Location()
	// 2026-03-31 北京时间：上一自然月应为 2026-02（不可与 AddDate(0,-1,0) 同式）
	ref := time.Date(2026, 3, 31, 9, 36, 58, 0, loc)
	got := PreviousMonthYYYYMM(ref)
	if got != "2026-02" {
		t.Fatalf("got %q, want 2026-02 (AddDate bug would yield 2026-03)", got)
	}
}
