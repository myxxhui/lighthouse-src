package service

import "context"

type ctxKeyLedgerRefresh int

const ledgerRefreshCtxKey ctxKeyLedgerRefresh = 1

// WithLedgerRefresh 标记本次请求需在计算五维前执行 FinOps 辅助同步（拉取 U/B 等到库）。
func WithLedgerRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, ledgerRefreshCtxKey, true)
}

// LedgerRefreshRequested 是否带 ledger_refresh 查询参数。
func LedgerRefreshRequested(ctx context.Context) bool {
	v, ok := ctx.Value(ledgerRefreshCtxKey).(bool)
	return ok && v
}
