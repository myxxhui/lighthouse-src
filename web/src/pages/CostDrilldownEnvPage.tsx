/**
 * 按环境云产品钻取页 [Ref: 01_设计 §按环境钻取 D9-4、D9-11 列表→详情时间范围一致]
 * 路由: /CostDrilldownEnvPage?env=POC|FAT|UAT|PROD&period=1d|7d|30d|...
 */
import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import { Card, Table, Select, Button, Space } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { costService, type EnvDrilldownApiItem } from '@/services/costService';
import { CURRENCY_SYMBOL } from '@/constants';
import type { CostTimeRange } from '@/types';
import { billingCalendarPartsFromNow } from '@/utils/billingCalendar';

const CATEGORY_OPTIONS = [
  { label: '全部', value: '' },
  { label: '计算资源', value: 'compute' },
  { label: '存储', value: 'storage' },
  { label: '网络', value: 'network' },
  { label: '安全', value: 'security' },
];

/** 与后端 reportTypeAndPeriodKey 口径一致：业务月 YYYY-MM 用 Asia/Shanghai [Ref: 01_设计 D9-7] */
function periodToReportTypeAndKey(period: string | null): { reportType: string; periodKey: string } {
  const { monthStr, prevMonthStr, quarterKey, prevQuarterKey, yearStr, prevYearStr } = billingCalendarPartsFromNow();
  if (!period || period === 'custom') {
    return { reportType: 'month', periodKey: monthStr };
  }
  const p = period as CostTimeRange;
  if (p === 'month') return { reportType: 'month', periodKey: monthStr };
  if (p === 'last_month') return { reportType: 'last_month', periodKey: prevMonthStr };
  if (p === 'quarter') return { reportType: 'quarter', periodKey: quarterKey };
  if (p === 'last_quarter') return { reportType: 'last_quarter', periodKey: prevQuarterKey };
  if (p === 'this_year') return { reportType: 'this_year', periodKey: yearStr };
  if (p === 'last_year') return { reportType: 'last_year', periodKey: prevYearStr };
  return { reportType: 'month', periodKey: monthStr };
}

const CostDrilldownEnvPage: React.FC = () => {
  const [searchParams] = useSearchParams();
  const env = searchParams.get('env') || 'POC';
  const period = searchParams.get('period');
  const { reportType, periodKey } = periodToReportTypeAndKey(period);
  const navigate = useNavigate();
  const [list, setList] = useState<EnvDrilldownApiItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [category, setCategory] = useState<string>('');

  useEffect(() => {
    setLoading(true);
    costService
      .getEnvDrilldown(env, { report_type: reportType, period_key: periodKey, category: category || undefined })
      .then(data => {
        setList(data);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [env, reportType, periodKey, category]);

  const columns = [
    { title: '产品编码', dataIndex: 'product_code', key: 'product_code' },
    { title: '产品名称', dataIndex: 'product_name', key: 'product_name', render: (v: string) => v || '-' },
    {
      title: '成本',
      dataIndex: 'cost',
      key: 'cost',
      render: (v: number) => `${CURRENCY_SYMBOL} ${Number(v).toLocaleString()}`,
    },
    { title: '分类', dataIndex: 'category', key: 'category' },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/CostOverviewPage')}>
          返回全域成本透视
        </Button>
      </Space>
      <Card
        title={`${env} 环境 - 云产品成本钻取`}
        extra={
          <Select
            value={category}
            onChange={setCategory}
            options={CATEGORY_OPTIONS}
            style={{ width: 120 }}
            placeholder="分类筛选"
          />
        }
      >
        <Table
          loading={loading}
          dataSource={list}
          columns={columns}
          rowKey="product_code"
          pagination={{ pageSize: 20 }}
          size="small"
        />
      </Card>
    </div>
  );
};

export default CostDrilldownEnvPage;
