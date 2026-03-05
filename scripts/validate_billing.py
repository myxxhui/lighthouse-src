#!/usr/bin/env python3
"""
阿里云账单一致性校验脚本
用途：对比 QueryBillOverview（月总额）与 QueryAccountBill（每日累计）的差异，定位数据误差根源。

使用方法：
  python3 validate_billing.py --month 2026-03
  python3 validate_billing.py --month 2026-02 --endpoint business.ap-southeast-1.aliyuncs.com

环境变量（与后端 Go 程序一致）：
  ALIBABA_CLOUD_ACCESS_KEY_ID     阿里云 AccessKeyId
  ALIBABA_CLOUD_ACCESS_KEY_SECRET 阿里云 AccessKeySecret
  CLOUD_BILLING_ENDPOINT          可选；国际站填 business.ap-southeast-1.aliyuncs.com

安装依赖：
  pip3 install alibabacloud_bssopenapi20171214
"""

import argparse
import os
import sys
import calendar
from datetime import date, timedelta
from typing import Optional

try:
    from alibabacloud_bssopenapi20171214 import models as bss_models
    from alibabacloud_bssopenapi20171214.client import Client as BssClient
    from alibabacloud_tea_openapi import models as open_api_models
except ImportError:
    print("缺少依赖，请运行：pip3 install alibabacloud_bssopenapi20171214")
    sys.exit(1)


def build_client(endpoint: Optional[str] = None) -> BssClient:
    ak = os.environ.get("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
    sk = os.environ.get("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
    if not ak or not sk:
        print("❌ 缺少环境变量 ALIBABA_CLOUD_ACCESS_KEY_ID / ALIBABA_CLOUD_ACCESS_KEY_SECRET")
        sys.exit(1)
    ep = endpoint or os.environ.get("CLOUD_BILLING_ENDPOINT", "") or "business.aliyuncs.com"
    cfg = open_api_models.Config(
        access_key_id=ak,
        access_key_secret=sk,
        endpoint=ep,
    )
    return BssClient(cfg)


def fetch_monthly_total(client: BssClient, billing_cycle: str) -> dict:
    """调用 QueryBillOverview 获取指定月份总额及分项（CashAmount 口径）。"""
    req = bss_models.QueryBillOverviewRequest(billing_cycle=billing_cycle)
    resp = client.query_bill_overview(req)
    data = resp.body.data
    total_cash = 0.0
    total_pretax = 0.0
    items = []
    if data and data.items and data.items.item:
        for it in data.items.item:
            cash = float(it.cash_amount or 0)
            pretax = float(it.pretax_amount or 0)
            item_type = it.item or "unknown"
            product_code = it.product_code or "unknown"
            total_cash += cash
            total_pretax += pretax
            items.append({
                "product_code": product_code,
                "item_type": item_type,
                "cash_amount": cash,
                "pretax_amount": pretax,
            })
    return {
        "billing_cycle": billing_cycle,
        "total_cash": total_cash,
        "total_pretax": total_pretax,
        "items": items,
    }


def fetch_daily_total(client: BssClient, billing_date: str) -> dict:
    """调用 QueryAccountBill（DAILY 粒度）获取指定日期的 CashAmount 总额。"""
    billing_cycle = billing_date[:7]
    page_size = 300
    page_num = 1
    total_cash = 0.0
    total_pretax = 0.0
    item_count = 0
    while True:
        req = bss_models.QueryAccountBillRequest(
            billing_cycle=billing_cycle,
            billing_date=billing_date,
            granularity="DAILY",
            is_group_by_product=True,
            page_num=page_num,
            page_size=page_size,
        )
        resp = client.query_account_bill(req)
        data = resp.body.data
        if not data or not data.items or not data.items.item:
            break
        for it in data.items.item:
            total_cash += float(it.cash_amount or 0)
            total_pretax += float(it.pretax_amount or 0)
            item_count += 1
        total_count = data.total_count or 0
        if page_num * page_size >= total_count:
            break
        page_num += 1
    return {
        "date": billing_date,
        "cash_amount": total_cash,
        "pretax_amount": total_pretax,
        "item_count": item_count,
    }


def run_validation(month: str, endpoint: Optional[str] = None) -> None:
    """执行完整的月度一致性校验并输出报告。"""
    client = build_client(endpoint)

    print(f"\n{'='*60}")
    print(f"  阿里云账单一致性校验  月份：{month}")
    print(f"{'='*60}")

    # 1. 获取月总额（QueryBillOverview）
    print(f"\n[1/3] 调用 QueryBillOverview 获取 {month} 月总额 ...")
    monthly = fetch_monthly_total(client, month)
    print(f"  月总额 CashAmount : {monthly['total_cash']:>12.2f}")
    print(f"  月总额 PretaxAmount: {monthly['total_pretax']:>12.2f}")
    print(f"  商品条目数         : {len(monthly['items'])}")

    # 2. 逐日获取日账单（QueryAccountBill）
    year, mon = int(month[:4]), int(month[5:7])
    today = date.today()
    last_day = calendar.monthrange(year, mon)[1]
    end_day = min(last_day, (today - timedelta(days=1)).day if (today.year, today.month) == (year, mon) else last_day)

    print(f"\n[2/3] 逐日调用 QueryAccountBill（1 日 → {end_day} 日）...")
    daily_rows = []
    day_sum_cash = 0.0
    day_sum_pretax = 0.0
    for d in range(1, end_day + 1):
        billing_date = f"{year:04d}-{mon:02d}-{d:02d}"
        daily = fetch_daily_total(client, billing_date)
        daily_rows.append(daily)
        day_sum_cash += daily["cash_amount"]
        day_sum_pretax += daily["pretax_amount"]
        status = "⚠️" if daily["cash_amount"] < 0 else ("  " if daily["cash_amount"] > 0 else "  ")
        print(f"  {billing_date}  Cash={daily['cash_amount']:>10.2f}  Pretax={daily['pretax_amount']:>10.2f}  items={daily['item_count']}  {status}")

    # 3. 对比报告
    print(f"\n[3/3] 对比报告")
    print(f"{'─'*60}")
    diff_cash = monthly["total_cash"] - day_sum_cash
    diff_pretax = monthly["total_pretax"] - day_sum_pretax
    diff_pct_cash = (abs(diff_cash) / monthly["total_cash"] * 100) if monthly["total_cash"] != 0 else 0
    diff_pct_pretax = (abs(diff_pretax) / monthly["total_pretax"] * 100) if monthly["total_pretax"] != 0 else 0

    print(f"  月总额  (QueryBillOverview)  CashAmount  : {monthly['total_cash']:>12.2f}")
    print(f"  日累计  (QueryAccountBill)   CashAmount  : {day_sum_cash:>12.2f}")
    print(f"  差额    (月 − 日累计)        CashAmount  : {diff_cash:>+12.2f}  ({diff_pct_cash:.2f}%)")
    print(f"")
    print(f"  月总额  (QueryBillOverview)  PretaxAmount: {monthly['total_pretax']:>12.2f}")
    print(f"  日累计  (QueryAccountBill)   PretaxAmount: {day_sum_pretax:>12.2f}")
    print(f"  差额    (月 − 日累计)        PretaxAmount: {diff_pretax:>+12.2f}  ({diff_pct_pretax:.2f}%)")

    # 找出日数据差异最大的天
    if daily_rows:
        max_diff_day = max(daily_rows, key=lambda x: abs(x["cash_amount"] - x["pretax_amount"]))
        neg_days = [r for r in daily_rows if r["cash_amount"] < 0]
        zero_days = [r for r in daily_rows if r["cash_amount"] == 0 and r["item_count"] == 0]

        print(f"\n  Cash vs Pretax 差异最大单日   : {max_diff_day['date']}  "
              f"Cash={max_diff_day['cash_amount']:.2f}  Pretax={max_diff_day['pretax_amount']:.2f}")
        if neg_days:
            print(f"  ⚠️  CashAmount 为负值的日期（实际现金退款）:")
            for r in neg_days:
                print(f"      {r['date']}  Cash={r['cash_amount']:.2f}")
        if zero_days:
            print(f"  ℹ️  无日账单数据的日期（items=0）:")
            for r in zero_days:
                print(f"      {r['date']}")

    # 结论
    print(f"\n{'─'*60}")
    if diff_pct_cash < 1.0:
        print(f"  ✅ CashAmount 口径：月日差异 < 1%，数据一致性良好。")
    else:
        print(f"  ❌ CashAmount 口径：月日差异 {diff_pct_cash:.2f}%，超过 1% 阈值，请人工核查。")
    print(f"{'='*60}\n")


def main() -> None:
    parser = argparse.ArgumentParser(description="阿里云账单一致性校验脚本（CashAmount 口径）")
    parser.add_argument("--month", required=True, help="要校验的月份，格式 YYYY-MM，如 2026-03")
    parser.add_argument("--endpoint", default=None, help="可选；国际站填 business.ap-southeast-1.aliyuncs.com")
    args = parser.parse_args()

    if len(args.month) != 7 or args.month[4] != "-":
        print("❌ --month 格式错误，应为 YYYY-MM（如 2026-03）")
        sys.exit(1)

    run_validation(args.month, args.endpoint)


if __name__ == "__main__":
    main()
