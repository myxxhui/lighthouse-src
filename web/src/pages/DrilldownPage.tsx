import React, { useEffect } from 'react';
import { Card, Descriptions, Button, Space, Alert, Spin } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import DrilldownNavigator from '@/components/DrilldownNavigator';
import EfficiencyChart from '@/components/EfficiencyChart';
import StatusIndicator from '@/components/StatusIndicator';
import { useAppStore } from '@/store';

const DrilldownPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const {
    currentDrilldownItem,
    loadingDrilldown,
    errorDrilldown,
    fetchDrilldownData,
    clearDrilldownPath,
  } = useAppStore();

  const type = searchParams.get('type') as 'namespace' | 'node' | 'workload' | 'pod';
  const id = searchParams.get('id');

  useEffect(() => {
    if (type && id) {
      fetchDrilldownData(type, id);
    }

    return () => {
      clearDrilldownPath();
    };
  }, [type, id, fetchDrilldownData, clearDrilldownPath]);

  const handleBack = () => {
    navigate(-1);
  };

  const handleHome = () => {
    navigate('/');
  };

  const renderDrilldownContent = () => {
    if (loadingDrilldown) {
      return (
        <div style={{ textAlign: 'center', padding: '40px' }}>
          <Spin indicator={<LoadingOutlined spin />} />
          <p>加载中...</p>
        </div>
      );
    }

    if (errorDrilldown) {
      return (
        <Alert
          message="加载钻取数据失败"
          description={errorDrilldown}
          type="error"
          showIcon
          action={
            <Button type="primary" onClick={() => type && id && fetchDrilldownData(type, id)}>
              重试
            </Button>
          }
        />
      );
    }

    if (!currentDrilldownItem) {
      return (
        <div style={{ textAlign: 'center', padding: '40px' }}>
          <p>暂无数据</p>
        </div>
      );
    }

    const getTypeName = (itemType: string) => {
      switch (itemType) {
        case 'namespace':
          return '命名空间';
        case 'node':
          return '节点';
        case 'workload':
          return '工作负载';
        case 'pod':
          return 'Pod';
        default:
          return itemType;
      }
    };

    // 基于效率分判断状态
    let status: 'healthy' | 'warning' | 'critical' = 'healthy';
    if (currentDrilldownItem.efficiency < 50) {
      status = 'critical';
    } else if (currentDrilldownItem.efficiency < 70) {
      status = 'warning';
    }

    return (
      <>
        <Card title={`${getTypeName(currentDrilldownItem.type)}: ${currentDrilldownItem.name}`}>
          <Descriptions column={{ xs: 1, sm: 2, md: 3 }} bordered>
            <Descriptions.Item label="ID">{currentDrilldownItem.id}</Descriptions.Item>
            <Descriptions.Item label="类型">
              {getTypeName(currentDrilldownItem.type)}
            </Descriptions.Item>
            <Descriptions.Item label="成本 (¥)">
              {currentDrilldownItem.cost.toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label="可优化空间 (¥)">
              {currentDrilldownItem.optimizableSpace.toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label="效率分">
              <Space>
                <EfficiencyChart
                  efficiency={currentDrilldownItem.efficiency}
                  size={40}
                  showLabel={false}
                />
                <span>{currentDrilldownItem.efficiency}%</span>
              </Space>
            </Descriptions.Item>
            <Descriptions.Item label="健康状态">
              <StatusIndicator status={status} showText={true} />
            </Descriptions.Item>
          </Descriptions>
        </Card>

        {currentDrilldownItem.children && currentDrilldownItem.children.length > 0 && (
          <Card title="子资源" style={{ marginTop: 16 }}>
            {currentDrilldownItem.children.map(child => (
              <Card
                key={child.id}
                style={{ marginBottom: 8 }}
                hoverable
                onClick={() => {
                  navigate(`/drilldown?type=${child.type}&id=${child.id}`);
                }}
              >
                <div
                  style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
                >
                  <div>
                    <strong>
                      {getTypeName(child.type)}: {child.name}
                    </strong>
                    <div style={{ fontSize: '12px', color: '#666' }}>
                      成本: ¥{child.cost.toLocaleString()} | 效率: {child.efficiency}%
                    </div>
                  </div>
                  <div>
                    <StatusIndicator
                      status={
                        child.efficiency < 50
                          ? 'critical'
                          : child.efficiency < 70
                            ? 'warning'
                            : 'healthy'
                      }
                    />
                  </div>
                </div>
              </Card>
            ))}
          </Card>
        )}
      </>
    );
  };

  return (
    <div>
      <DrilldownNavigator onBack={handleBack} onHome={handleHome} />
      {renderDrilldownContent()}
    </div>
  );
};

export default DrilldownPage;
