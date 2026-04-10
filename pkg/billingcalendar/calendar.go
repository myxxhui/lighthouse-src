// Package billingcalendar 业务账期自然月计算，与 Asia/Shanghai 日历配合使用。[Ref: 03_Phase6/03_前端全域成本透视/01_设计]
package billingcalendar

import "time"

// PreviousMonthYYYYMM 返回 ref 所在自然月的「上一自然月」YYYY-MM（与 ref 同一 Location）。
//
// 注意：不可对 ref 直接使用 AddDate(0,-1,0) 再 Format：在月末（如 3/31）Go 会规范到同月内其他日（如 3/3），
// Format("2006-01") 仍为上个月历月，导致「上月」错算成当月。[Ref: Go time.AddDate 文档]
func PreviousMonthYYYYMM(ref time.Time) string {
	first := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	return first.AddDate(0, -1, 0).Format("2006-01")
}
