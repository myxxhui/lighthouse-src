import React, { useEffect } from 'react';
import { Card, Statistic, Row, Col, Alert, Spin, Tabs } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import TrendChart from '@/components/TrendChart';
import { useAppStore } from '@/store';

const ROITrendSection: React.FC = () => {
  const { roiTrends, loadingROI, errorROI, fetchROITrends, useMockData } = useAppStore();

  useEffect(() => {
    fetchROITrends();
  }, [fetchROITrends]);

  const renderSummaryStats = () => {
    if (!roiTrends || roiTrends.length === 0) {
      return null;
    }

    const latest = roiTrends[roiTrends.length - 1];
    const previous = roiTrends.length > 1 ? roiTrends[roiTrends.length - 2] : null;

    const valueChange = previous ? ((latest.value - previous.value) / previous.value) * 100 : 0;
    const costChange = previous ? ((latest.cost - previous.cost) / previous.cost) * 100 : 0;
    const efficiencyChange = previous ? latest.efficiency - previous.efficiency : 0;

    return (
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={12} md={8}>
          <Card>
            <Statistic
              title="当前价值"
              value={latest.value}
              prefix="¥"
              formatter={value => Number(value).toLocaleString()}
              valueStyle={{ color: valueChange >= 0 ? '#52c41a' : '#ff4d4f' }}
              suffix={
                previous && (
                  <span
                    style={{ fontSize: '12px', color: valueChange >= 0 ? '#52c41a' : '#ff4d4f' }}
                  >
                    {valueChange >= 0 ? '+' : ''}
                    {valueChange.toFixed(2)}%
                  </span>
                )
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8}>
          <Card>
            <Statistic
              title="当前成本"
              value={latest.cost}
              prefix="¥"
              formatter={value => Number(value).toLocaleString()}
              valueStyle={{ color: costChange <= 0 ? '#52c41a' : '#ff4d4f' }}
              suffix={
                previous && (
                  <span
                    style={{ fontSize: '12px', color: costChange <= 0 ? '#52c41a' : '#ff4d4f' }}
                  >
                    {costChange <= 0 ? '' : '+'}
                    {costChange.toFixed(2)}%
                  </span>
                )
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8}>
          <Card>
            <Statistic
              title="当前效率分"
              value={latest.efficiency}
              valueStyle={{ color: efficiencyChange >= 0 ? '#52c41a' : '#ff4d4f' }}
              suffix={
                <span
                  style={{ fontSize: '12px', color: efficiencyChange >= 0 ? '#52c41a' : '#ff4d4f' }}
                >
                  {efficiencyChange >= 0 ? '+' : ''}
                  {efficiencyChange.toFixed(2)}%
                </span>
              }
            />
          </Card>
        </Col>
      </Row>
    );
  };

  const renderCharts = () => {
    if (!roiTrends || roiTrends.length === 0) {
      return (
        <div style={{ textAlign: 'center', padding: '40px' }}>
          <p>暂无趋势数据</p>
        </div>
      );
    }

    const thirtyDaysAgo = new Date();
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);
    const recentTrends = roiTrends.filter(trend => {
      const trendDate = new Date(trend.date);
      return trendDate >= thirtyDaysAgo;
    });

    return (
      <Tabs defaultActiveKey="1">
        <Tabs.TabPane tab="价值与成本趋势" key="1">
          <TrendChart data={recentTrends} showEfficiency={false} height={400} />
        </Tabs.TabPane>
        <Tabs.TabPane tab="效率分趋势" key="2">
          <TrendChart data={recentTrends} showEfficiency={true} height={400} />
        </Tabs.TabPane>
        <Tabs.TabPane tab="完整趋势" key="3">
          <TrendChart data={roiTrends} showEfficiency={true} height={400} />
        </Tabs.TabPane>
      </Tabs>
    );
  };

  if (loadingROI) {
    return (
      <div style={{ textAlign: 'center', padding: '40px' }}>
        <Spin indicator={<LoadingOutlined spin />} />
        <p>加载ROI趋势中...</p>
      </div>
    );
  }

  if (errorROI) {
    return (
      <Alert
        message="加载ROI趋势失败"
        description={errorROI}
        type="error"
        showIcon
        style={{ marginBottom: 16 }}
      />
    );
  }

  return (
    <div>
      {useMockData && (
        <Alert message="当前使用Mock数据" type="info" style={{ marginBottom: 16 }} />
      )}
      {renderSummaryStats()}
      <Card title="趋势分析">{renderCharts()}</Card>
    </div>
  );
};

export default ROITrendSection;
