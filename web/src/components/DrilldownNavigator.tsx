import React from 'react';
import { Breadcrumb, Button, Space } from 'antd';
import { HomeOutlined, ArrowLeftOutlined } from '@ant-design/icons';
import { useAppStore } from '@/store';

const DIMENSION_LABELS: Record<string, string> = {
  compute: '算力',
  storage: '存储',
  network: '网络',
};

interface DrilldownNavigatorProps {
  onBack?: () => void;
  onHome?: () => void;
  /** 当前资源维度，用于面包屑首项 */
  dimension?: string;
}

const DrilldownNavigator: React.FC<DrilldownNavigatorProps> = ({ onBack, onHome, dimension }) => {
  const { drilldownPath, clearDrilldownPath, selectedDimension } = useAppStore();
  const dim = dimension ?? selectedDimension;

  const handleBack = () => {
    if (onBack) {
      onBack();
    } else if (drilldownPath.length > 0) {
      clearDrilldownPath();
    }
  };

  const handleHome = () => {
    if (onHome) {
      onHome();
    } else {
      clearDrilldownPath();
    }
  };

  const typeNameMap: Record<string, string> = {
    namespace: '命名空间',
    node: '节点',
    workload: '工作负载',
    pod: 'Pod',
    storage_class: '存储类',
    pvc: 'PVC',
    volume: '卷',
    service: '服务',
    ingress: 'Ingress',
    lb: '负载均衡',
    traffic_type: '流量类型',
  };

  const breadcrumbItems = [
    ...(dim ? [{ title: DIMENSION_LABELS[dim] ?? dim, key: 'dim' }] : []),
    ...drilldownPath.map((pathItem, index) => {
      const [type, id] = pathItem.split(':');
      const typeName = typeNameMap[type] ?? type;
      return {
        title: `${typeName}: ${id}`,
        key: `path-${index}`,
      };
    }),
  ];

  return (
    <Space size="middle" style={{ marginBottom: 16 }}>
      <Button
        icon={<ArrowLeftOutlined />}
        onClick={handleBack}
        disabled={drilldownPath.length === 0}
      >
        返回
      </Button>
      <Button icon={<HomeOutlined />} onClick={handleHome}>
        首页
      </Button>
      {breadcrumbItems.length > 0 && <Breadcrumb items={breadcrumbItems} />}
    </Space>
  );
};

export default DrilldownNavigator;
