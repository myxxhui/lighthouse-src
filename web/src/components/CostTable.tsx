import React from 'react';
import { Table, Tag, Space, Button } from 'antd';
import type { NamespaceCost } from '@/types';
import EfficiencyChart from '@/components/EfficiencyChart';
import StatusIndicator from '@/components/StatusIndicator';

interface CostTableProps {
  data: NamespaceCost[];
  loading?: boolean;
  onRowClick?: (record: NamespaceCost) => void;
  showRecommendations?: boolean;
}

const CostTable: React.FC<CostTableProps> = ({
  data,
  loading = false,
  onRowClick,
  showRecommendations = true,
}) => {
  const columns = [
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
      render: (text: string) => <strong>{text}</strong>,
    },
    {
      title: '成本 (¥)',
      dataIndex: 'cost',
      key: 'cost',
      render: (value: number) => value.toLocaleString(),
      sorter: (a: NamespaceCost, b: NamespaceCost) => a.cost - b.cost,
    },
    {
      title: '可优化空间 (¥)',
      dataIndex: 'optimizableSpace',
      key: 'optimizableSpace',
      render: (value: number) => <Tag color="warning">{value.toLocaleString()}</Tag>,
      sorter: (a: NamespaceCost, b: NamespaceCost) => a.optimizableSpace - b.optimizableSpace,
    },
    {
      title: '效率分 (%)',
      dataIndex: 'efficiency',
      key: 'efficiency',
      render: (value: number) => (
        <Space>
          <EfficiencyChart efficiency={value} size={40} showLabel={false} />
          <span>{value}%</span>
        </Space>
      ),
      sorter: (a: NamespaceCost, b: NamespaceCost) => a.efficiency - b.efficiency,
    },
    {
      title: '资源使用率',
      key: 'resourceUsage',
      render: (_: any, record: NamespaceCost) => (
        <Space direction="vertical" size="small">
          <div>CPU: {record.resourceUsage.cpu}%</div>
          <div>内存: {record.resourceUsage.memory}%</div>
          <div>存储: {record.resourceUsage.storage}%</div>
        </Space>
      ),
    },
    {
      title: '健康状态',
      key: 'status',
      render: (_: any, record: NamespaceCost) => {
        // 基于效率分判断状态
        let status: 'healthy' | 'warning' | 'critical' = 'healthy';
        if (record.efficiency < 50) {
          status = 'critical';
        } else if (record.efficiency < 70) {
          status = 'warning';
        }
        return <StatusIndicator status={status} />;
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: NamespaceCost) => (
        <Button type="link" onClick={() => onRowClick && onRowClick(record)}>
          详情
        </Button>
      ),
    },
  ];

  const expandedRowRender = (record: NamespaceCost) => {
    if (!showRecommendations || !record.recommendations?.length) {
      return null;
    }

    return (
      <div style={{ padding: '16px 0' }}>
        <h4>优化建议:</h4>
        <ul>
          {record.recommendations.map((rec, index) => (
            <li key={index}>{rec}</li>
          ))}
        </ul>
      </div>
    );
  };

  return (
    <Table
      dataSource={data}
      columns={columns}
      loading={loading}
      rowKey="namespace"
      onRow={record => ({
        onClick: () => onRowClick && onRowClick(record),
      })}
      expandable={{
        expandedRowRender,
        rowExpandable: record => showRecommendations && !!record.recommendations?.length,
      }}
      pagination={{
        pageSize: 10,
        showSizeChanger: true,
        pageSizeOptions: ['10', '20', '50'],
      }}
    />
  );
};

export default CostTable;
