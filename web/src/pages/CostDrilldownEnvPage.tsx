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

const CATEGORY_OPTIONS = [
  { label: '全部', value: '' },
  { label: '计算资源', value: 'compute' },
  { label: '存储', value: 'storage' },
  { label: '网络', value: 'network' },
  { label: '安全', value: 'security' },
];

/** 与后端 reportTypeAndPeriodKey 口径一致：7d/30d/90d 结束日为昨日 [Ref: 01_设计 D9-7] */
function periodToReportTypeAndKey(period: string | null): { reportType: string; periodKey: string } {
  const now = new Date();
  const yyyy = now.getFullYear();
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  if (!period || period === 'custom') {
    return { reportType: 'month', periodKey: `${yyyy}-${mm}` };
  }
  const p = period as CostTimeRange;
  if (p === 'month') return { reportType: 'month', periodKey: `${yyyy}-${mm}` };
  if (p === 'last_month') {
    const prevMonth = now.getMonth() === 0 ? 12 : now.getMonth();
    const prevYear = now.getMonth() === 0 ? yyyy - 1 : yyyy;
    return { reportType: 'last_month', periodKey: `${prevYear}-${String(prevMonth).padStart(2, '0')}` };
  }
  if (p === 'quarter' || p === 'last_quarter') return { reportType: p, periodKey: `${yyyy}-Q${Math.ceil((now.getMonth() + 1) / 3)}` };
  if (p === 'this_year') return { reportType: 'this_year', periodKey: String(yyyy) };
  if (p === 'last_year') return { reportType: 'last_year', periodKey: String(yyyy - 1) };
  return { reportType: 'month', periodKey: `${yyyy}-${mm}` };
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
