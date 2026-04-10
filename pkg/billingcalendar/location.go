package billingcalendar

import (
	"sync"
	"time"
)

var (
	billingLocationOnce sync.Once
	billingLocation     *time.Location
)

// Location 返回业务账单锚点时区：优先 IANA「Asia/Shanghai」。
// 若运行环境无 zoneinfo（如精简 Alpine 镜像中 LoadLocation 失败），退化为 UTC+8 固定偏移，
// 避免 time.In(nil) 触发 panic；与北京时间无夏令时一致。[Ref: 03_Phase6/01_FinOps ETL 账期]
func Location() *time.Location {
	billingLocationOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil || loc == nil {
			billingLocation = time.FixedZone("Asia/Shanghai", 8*3600)
		} else {
			billingLocation = loc
		}
	})
	return billingLocation
}
