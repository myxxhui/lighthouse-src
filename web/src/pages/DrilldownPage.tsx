import React, { useEffect } from 'react';
import { Card, Descriptions, Button, Space, Alert, Spin, Progress, Segmented } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import type { ResourceDimension } from '@/types';
import DrilldownNavigator from '@/components/DrilldownNavigator';
import EfficiencyChart from '@/components/EfficiencyChart';
import StatusIndicator from '@/components/StatusIndicator';
import { useAppStore } from '@/store';

const DIMENSION_OPTIONS: { value: ResourceDimension; label: string }[] = [
  { value: 'compute', label: '算力' },
  { value: 'storage', label: '存储' },
  { value: 'network', label: '网络' },
];

const DrilldownPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const {
    currentDrilldownItem,
    loadingDrilldown,
    errorDrilldown,
    namespaceCosts,
    loadingNamespaceCosts,
    selectedDimension,
    setSelectedDimension,
    fetchDrilldownData,
    fetchNamespaceCosts,
    clearDrilldownPath,
  } = useAppStore();

  const type = searchParams.get('type') ?? undefined;
  const id = searchParams.get('id') ?? undefined;
  const dimensionParam = searchParams.get('dimension') as ResourceDimension | null;
  const dimension = dimensionParam && ['compute', 'storage', 'network'].includes(dimensionParam)
    ? dimensionParam
    : selectedDimension;

  useEffect(() => {
    if (dimensionParam && dimensionParam !== selectedDimension) {
      setSelectedDimension(dimensionParam as ResourceDimension);
    }
  }, [dimensionParam, selectedDimension, setSelectedDimension]);

  useEffect(() => {
    if (type && id) {
      fetchDrilldownData(type, id, dimension);
    } else {
      fetchNamespaceCosts();
    }

    return () => {
      clearDrilldownPath();
    };
  }, [type, id, dimension, fetchDrilldownData, fetchNamespaceCosts, clearDrilldownPath]);

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
            <Button type="primary" onClick={() => type && id && fetchDrilldownData(type, id, dimension)}>
              重试
            </Button>
          }
        />
      );
    }

    // 无 type/id 时展示入口视图或引导
    if (!type || !id) {
      if (loadingNamespaceCosts && !namespaceCosts?.length) {
        return (
          <div style={{ textAlign: 'center', padding: '40px' }}>
            <Spin indicator={<LoadingOutlined spin />} />
            <p>加载命名空间...</p>
          </div>
        );
      }
      if (namespaceCosts && namespaceCosts.length > 0) {
        return (
          <Card title="选择命名空间开始下钻">
            <p style={{ marginBottom: 12, color: '#666' }}>
              选择资源维度后，从下方选择命名空间查看成本逐层下钻，或前往全域成本透视查看总览。
            </p>
            <div style={{ marginBottom: 16 }}>
              <span style={{ marginRight: 8 }}>资源维度：</span>
              <Segmented
                options={DIMENSION_OPTIONS}
                value={selectedDimension}
                onChange={(v) => setSelectedDimension(v as ResourceDimension)}
              />
            </div>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              {namespaceCosts.map(ns => (
                <Card
                  key={ns.namespace}
                  hoverable
                  onClick={() =>
                    navigate(
                      `/DrilldownPage?dimension=${selectedDimension}&type=namespace&id=${encodeURIComponent(ns.namespace)}`,
                    )
                  }
                  style={{ marginBottom: 0 }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <strong>{ns.namespace}</strong>
                    <span>
                      成本 ¥{ns.cost.toLocaleString()} · 效率 {ns.efficiency}%
                    </span>
                  </div>
                </Card>
              ))}
            </Space>
            <div style={{ marginTop: 16 }}>
              <Button type="primary" onClick={() => navigate('/CostOverviewPage')}>
                前往全域成本透视
              </Button>
            </div>
          </Card>
        );
      }
      return (
        <Alert
          message="请选择下钻起点"
          description="请从「全域成本透视」选择命名空间开始下钻，或等待数据加载后在此选择。"
          type="info"
          showIcon
          action={
            <Button type="primary" onClick={() => navigate('/CostOverviewPage')}>
              前往全域成本透视
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
        case 'storage_class':
          return '存储类';
        case 'pvc':
          return 'PVC';
        case 'volume':
          return '卷';
        case 'service':
          return '服务';
        case 'ingress':
          return 'Ingress';
        case 'lb':
          return '负载均衡';
        case 'traffic_type':
          return '流量类型';
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
          {currentDrilldownItem.costBreakdown && (
            <div style={{ marginTop: 16 }}>
              <div style={{ marginBottom: 8, fontWeight: 500 }}>成本构成</div>
              <Space direction="vertical" style={{ width: '100%' }} size="small">
                <div>
                  <span style={{ display: 'inline-block', width: 72 }}>算力(CPU)</span>
                  <Progress
                    percent={Math.min(100, Math.round((currentDrilldownItem.costBreakdown.cpu / currentDrilldownItem.cost) * 100))}
                    size="small"
                    showInfo={true}
                    format={(p) => `¥${currentDrilldownItem.costBreakdown!.cpu.toLocaleString()} (${p}%)`}
                  />
                </div>
                <div>
                  <span style={{ display: 'inline-block', width: 72 }}>内存</span>
                  <Progress
                    percent={Math.min(100, Math.round((currentDrilldownItem.costBreakdown.memory / currentDrilldownItem.cost) * 100))}
                    size="small"
                    showInfo={true}
                    format={(p) => `¥${currentDrilldownItem.costBreakdown!.memory.toLocaleString()} (${p}%)`}
                  />
                </div>
                <div>
                  <span style={{ display: 'inline-block', width: 72 }}>存储</span>
                  <Progress
                    percent={Math.min(100, Math.round((currentDrilldownItem.costBreakdown.storage / currentDrilldownItem.cost) * 100))}
                    size="small"
                    showInfo={true}
                    format={(p) => `¥${currentDrilldownItem.costBreakdown!.storage.toLocaleString()} (${p}%)`}
                  />
                </div>
                <div>
                  <span style={{ display: 'inline-block', width: 72 }}>网络</span>
                  <Progress
                    percent={Math.min(100, Math.round((currentDrilldownItem.costBreakdown.network / currentDrilldownItem.cost) * 100))}
                    size="small"
                    showInfo={true}
                    format={(p) => `¥${currentDrilldownItem.costBreakdown!.network.toLocaleString()} (${p}%)`}
                  />
                </div>
              </Space>
            </div>
          )}
        </Card>

        {currentDrilldownItem.children && currentDrilldownItem.children.length > 0 && (
          <Card title="子资源" style={{ marginTop: 16 }}>
            {currentDrilldownItem.children.map(child => (
              <Card
                key={child.id}
                style={{ marginBottom: 8 }}
                hoverable
                onClick={() => {
                  navigate(
                    `/DrilldownPage?dimension=${dimension}&type=${encodeURIComponent(child.type)}&id=${encodeURIComponent(child.id)}`,
                  );
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
      <DrilldownNavigator onBack={handleBack} onHome={handleHome} dimension={dimension} />
      {renderDrilldownContent()}
    </div>
  );
};

export default DrilldownPage;
