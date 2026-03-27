// Package etl — RunFullETLCycle 供定时任务与 FinOps 主动同步 Job 复用。[Ref: 03_Phase6/01_FinOps 主动同步]
package etl

import (
	"context"
	"log"
	"time"
)

// RunFullETLCycle 执行一次账单 ETL 周期：全量检查 → 不满足则全量回填，满足则仅增量；再执行流水线与对账。
// 返回的 warnings 为可继续类问题（回填/对账失败等）；err 非空表示 RunPipeline 失败（与 main 定时任务语义一致）。
// [Ref: 04_实践 cmd/server/main.go runBillingETLCycle]
func RunFullETLCycle(ctx context.Context, worker *BillingWorker, maxDuration time.Duration) (warnings []string, err error) {
	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()
	needFull, e := worker.NeedsFullBackfill(ctx)
	if e != nil {
		log.Printf("WARN: billing full check failed, will run full backfill: %v", e)
		warnings = append(warnings, "full_check:"+e.Error())
		needFull = true
	}
	if needFull {
		log.Printf("billing ETL: full data check failed, running full backfill (daily: current month; monthly: 5 years)")
		if e := worker.RunFullBackfill(ctx); e != nil {
			log.Printf("WARN: billing full backfill failed: %v", e)
			warnings = append(warnings, "full_backfill:"+e.Error())
			if worker.OnPipelineFailAlert != nil {
				worker.OnPipelineFailAlert("full_backfill", e)
			}
		}
	}
	if e := worker.RunPipeline(ctx); e != nil {
		log.Printf("WARN: billing ETL pipeline run failed: %v", e)
		if worker.OnPipelineFailAlert != nil {
			worker.OnPipelineFailAlert("pipeline", e)
		}
		return warnings, e
	}
	if e := worker.RunReconcile(ctx); e != nil {
		log.Printf("WARN: billing reconcile failed: %v", e)
		warnings = append(warnings, "reconcile:"+e.Error())
	}
	return warnings, nil
}
