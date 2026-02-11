import React, { useEffect } from 'react';
import { Card, Row, Col, Statistic, Switch, Space, Alert } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import CostTable from '@/components/CostTable';
import EfficiencyChart from '@/components/EfficiencyChart';
import { useAppStore } from '@/store';

const CostOverviewPage: React.FC = () => {
  const {
    globalCostMetrics,
    namespaceCosts,
    loadingGlobalMetrics,
    loadingNamespaceCosts,
    errorGlobalMetrics,
    errorNamespaceCosts,
    useMockData,
    fetchGlobalCostMetrics,
    fetchNamespaceCosts,
    setUseMockData,
  } = useAppStore();

  useEffect(() => {
    fetchGlobalCostMetrics();
    fetchNamespaceCosts();
  }, [fetchGlobalCostMetrics, fetchNamespaceCosts]);

  const handleRowClick = (record: any) => {
    // TODO: 导航到钻取页面
    console.log('Navigate to drilldown for:', record.namespace);
  };

  const renderGlobalMetrics = () => {
    if (loadingGlobalMetrics) {
      return (
        <Card loading={true}>
          <LoadingOutlined />
        </Card>
      );
    }

    if (errorGlobalMetrics) {
      return (
        <Alert message="加载全局指标失败" description={errorGlobalMetrics} type="error" showIcon />
      );
    }

    if (!globalCostMetrics) {
      return <Card>暂无数据</Card>;
    }

    return (
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="总账单成本"
              value={globalCostMetrics.totalBillableCost}
              prefix="¥"
              formatter={value => Number(value).toLocaleString()}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="可优化空间"
              value={globalCostMetrics.totalOptimizableSpace}
              prefix="¥"
              formatter={value => Number(value).toLocaleString()}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic title="全局效率分" value={globalCostMetrics.globalEfficiency} suffix="%" />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <div
              style={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
              }}
            >
              <EfficiencyChart efficiency={globalCostMetrics.globalEfficiency} size={80} />
            </div>
          </Card>
        </Col>
      </Row>
    );
  };

  const renderDomainBreakdown = () => {
    if (!globalCostMetrics?.domainBreakdown?.length) {
      return null;
    }

    return (
      <Card title="领域成本分解" style={{ marginTop: 16 }}>
        <Row gutter={[16, 16]}>
          {globalCostMetrics.domainBreakdown.map((domain, index) => (
            <Col key={index} xs={24} sm={12} md={8} lg={6}>
              <Card size="small">
                <Statistic
                  title={domain.domain}
                  value={domain.cost}
                  prefix="¥"
                  formatter={value => Number(value).toLocaleString()}
                  suffix={
                    <div style={{ marginTop: 8 }}>
                      <small>效率: {domain.efficiency}%</small>
                    </div>
                  }
                />
              </Card>
            </Col>
          ))}
        </Row>
      </Card>
    );
  };

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16,
        }}
      >
        <h2>全域成本透视</h2>
        <Space>
          <span>使用Mock数据</span>
          <Switch
            checked={useMockData}
            onChange={setUseMockData}
            checkedChildren="是"
            unCheckedChildren="否"
          />
        </Space>
      </div>

      {renderGlobalMetrics()}
      {renderDomainBreakdown()}

      <Card title="命名空间成本明细" style={{ marginTop: 16 }}>
        {loadingNamespaceCosts ? (
          <div style={{ textAlign: 'center', padding: '20px' }}>
            <LoadingOutlined spin /> 加载中...
          </div>
        ) : errorNamespaceCosts ? (
          <Alert
            message="加载命名空间成本失败"
            description={errorNamespaceCosts}
            type="error"
            showIcon
          />
        ) : namespaceCosts?.length ? (
          <CostTable data={namespaceCosts} onRowClick={handleRowClick} />
        ) : (
          <div style={{ textAlign: 'center', padding: '20px' }}>暂无命名空间数据</div>
        )}
      </Card>
    </div>
  );
};

export default CostOverviewPage;
