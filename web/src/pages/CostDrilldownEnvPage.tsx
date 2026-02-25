/**
 * 按环境云产品钻取页 [Ref: 01_设计 §按环境钻取 D9-4]
 * 路由: /CostDrilldownEnvPage?env=POC|FAT|UAT|PROD
 */
import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import { Card, Table, Select, Button, Space } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { costService, type EnvDrilldownApiItem } from '@/services/costService';
import { CURRENCY_SYMBOL } from '@/constants';

const CATEGORY_OPTIONS = [
  { label: '全部', value: '' },
  { label: '计算', value: 'compute' },
  { label: '网络', value: 'network' },
  { label: '存储', value: 'storage' },
  { label: '安全', value: 'security' },
  { label: '其他', value: 'other' },
];

const CostDrilldownEnvPage: React.FC = () => {
  const [searchParams] = useSearchParams();
  const env = searchParams.get('env') || 'POC';
  const navigate = useNavigate();
  const [list, setList] = useState<EnvDrilldownApiItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [category, setCategory] = useState<string>('');

  useEffect(() => {
    setLoading(true);
    const reportType = '30d';
    const periodKey = new Date().toISOString().slice(0, 10);
    costService
      .getEnvDrilldown(env, { report_type: reportType, period_key: periodKey, category: category || undefined })
      .then(data => {
        setList(data);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [env, category]);

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
