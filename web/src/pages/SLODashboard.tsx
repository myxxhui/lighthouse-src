import React, { useEffect, useState } from 'react';
import { Card, Table, Tag, Statistic, Row, Col, Alert, Spin, Space, Segmented } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import StatusIndicator from '@/components/StatusIndicator';
import { useAppStore } from '@/store';
import type { SLOScope } from '@/types';

const SCOPE_LABELS: Record<SLOScope, string> = {
  global: '全域',
  domain: '域',
  service: '服务',
  pod: 'Pod',
};

const SLODashboard: React.FC = () => {
  const { sloStatus, loadingSLO, errorSLO, fetchSLOStatus, useMockData } = useAppStore();
  const [scope, setScope] = useState<SLOScope>('service');

  useEffect(() => {
    fetchSLOStatus(scope);
  }, [scope, fetchSLOStatus]);

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'healthy':
        return <Tag color="success">健康</Tag>;
      case 'warning':
        return <Tag color="warning">警告</Tag>;
      case 'critical':
        return <Tag color="error">严重</Tag>;
      default:
        return <Tag>未知</Tag>;
    }
  };

  const columns = [
    {
      title: '名称',
      key: 'name',
      render: (_: unknown, record: { scopeName?: string; serviceName: string }) => (
        <strong>{record.scopeName ?? record.serviceName}</strong>
      ),
    },
    {
      title: '健康状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Space>
          <StatusIndicator status={status as any} showText={true} />
          {getStatusTag(status)}
        </Space>
      ),
    },
    {
      title: '可用性 (%)',
      dataIndex: 'uptime',
      key: 'uptime',
      render: (value: number) => `${value.toFixed(2)}%`,
      sorter: (a: any, b: any) => a.uptime - b.uptime,
    },
    {
      title: '响应时间 (ms)',
      dataIndex: 'responseTime',
      key: 'responseTime',
      render: (value: number) => value,
      sorter: (a: any, b: any) => a.responseTime - b.responseTime,
    },
    {
      title: '错误率 (%)',
      dataIndex: 'errorRate',
      key: 'errorRate',
      render: (value: number) => `${value.toFixed(2)}%`,
      sorter: (a: any, b: any) => a.errorRate - b.errorRate,
    },
  ];

  const drillDownScope = (current: SLOScope): SLOScope | null => {
    if (current === 'global') return 'domain';
    if (current === 'domain') return 'service';
    if (current === 'service') return 'pod';
    return null;
  };

  const renderSummaryStats = () => {
    if (!sloStatus || sloStatus.length === 0) {
      return null;
    }

    const totalItems = sloStatus.length;
    const healthyCount = sloStatus.filter(s => s.status === 'healthy').length;
    const warningCount = sloStatus.filter(s => s.status === 'warning').length;
    const criticalCount = sloStatus.filter(s => s.status === 'critical').length;

    const avgUptime = sloStatus.reduce((sum, s) => sum + s.uptime, 0) / totalItems;
    const avgResponseTime = sloStatus.reduce((sum, s) => sum + s.responseTime, 0) / totalItems;
    const avgErrorRate = sloStatus.reduce((sum, s) => sum + s.errorRate, 0) / totalItems;

    return (
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title={`总${SCOPE_LABELS[scope]}数`} value={totalItems} />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title={`健康${SCOPE_LABELS[scope]}`} value={healthyCount} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title={`警告${SCOPE_LABELS[scope]}`} value={warningCount} valueStyle={{ color: '#faad14' }} />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={`严重${SCOPE_LABELS[scope]}`}
              value={criticalCount}
              valueStyle={{ color: '#ff4d4f' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="平均可用性" value={avgUptime.toFixed(2)} suffix="%" />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="平均响应时间" value={Math.round(avgResponseTime)} suffix="ms" />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="平均错误率" value={avgErrorRate.toFixed(2)} suffix="%" />
          </Card>
        </Col>
      </Row>
    );
  };

  return (
    <div>
      <h2>SLO健康监控</h2>

      {useMockData && <Alert message="当前使用Mock数据" type="info" style={{ marginBottom: 16 }} />}

      {loadingSLO ? (
        <div style={{ textAlign: 'center', padding: '40px' }}>
          <Spin indicator={<LoadingOutlined spin />} />
          <p>加载SLO状态中...</p>
        </div>
      ) : errorSLO ? (
        <Alert
          message="加载SLO状态失败"
          description={errorSLO}
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
        />
      ) : (
        <>
          <div style={{ marginBottom: 16 }}>
            <Space align="center">
              <span>层级：</span>
              <Segmented
                value={scope}
                onChange={v => setScope(v as SLOScope)}
                options={[
                  { label: '全域', value: 'global' },
                  { label: '域', value: 'domain' },
                  { label: '服务', value: 'service' },
                  { label: 'Pod', value: 'pod' },
                ]}
              />
            </Space>
          </div>
          {renderSummaryStats()}
          <Card title={`${SCOPE_LABELS[scope]} SLO 详情`}>
            {sloStatus?.length ? (
              <Table
                dataSource={sloStatus}
                columns={columns}
                rowKey={(r) => r.scopeId ?? r.serviceName}
                onRow={(record) => {
                  const next = drillDownScope(scope);
                  return {
                    style: next ? { cursor: 'pointer' } : undefined,
                    onClick: () => next && setScope(next),
                  };
                }}
                pagination={{
                  pageSize: 10,
                  showSizeChanger: true,
                  pageSizeOptions: ['10', '20', '50'],
                }}
              />
            ) : (
              <div style={{ textAlign: 'center', padding: '20px' }}>暂无SLO数据</div>
            )}
          </Card>
        </>
      )}
    </div>
  );
};

export default SLODashboard;
