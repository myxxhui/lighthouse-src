import React, { useEffect } from 'react';
import { Card, Table, Tag, Statistic, Row, Col, Alert, Spin, Space } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import StatusIndicator from '@/components/StatusIndicator';
import { useAppStore } from '@/store';

const SLODashboard: React.FC = () => {
  const { sloStatus, loadingSLO, errorSLO, fetchSLOStatus, useMockData } = useAppStore();

  useEffect(() => {
    fetchSLOStatus();
  }, [fetchSLOStatus]);

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
      title: '服务名称',
      dataIndex: 'serviceName',
      key: 'serviceName',
      render: (text: string) => <strong>{text}</strong>,
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

  const renderSummaryStats = () => {
    if (!sloStatus || sloStatus.length === 0) {
      return null;
    }

    const totalServices = sloStatus.length;
    const healthyServices = sloStatus.filter(s => s.status === 'healthy').length;
    const warningServices = sloStatus.filter(s => s.status === 'warning').length;
    const criticalServices = sloStatus.filter(s => s.status === 'critical').length;

    const avgUptime = sloStatus.reduce((sum, s) => sum + s.uptime, 0) / totalServices;
    const avgResponseTime = sloStatus.reduce((sum, s) => sum + s.responseTime, 0) / totalServices;
    const avgErrorRate = sloStatus.reduce((sum, s) => sum + s.errorRate, 0) / totalServices;

    return (
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="总服务数" value={totalServices} />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="健康服务" value={healthyServices} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="警告服务" value={warningServices} valueStyle={{ color: '#faad14' }} />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="严重服务"
              value={criticalServices}
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
          {renderSummaryStats()}
          <Card title="服务SLO详情">
            {sloStatus?.length ? (
              <Table
                dataSource={sloStatus}
                columns={columns}
                rowKey="serviceName"
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
