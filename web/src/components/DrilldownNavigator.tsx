import React from 'react';
import { Breadcrumb, Button, Space } from 'antd';
import { HomeOutlined, ArrowLeftOutlined } from '@ant-design/icons';
import { useAppStore } from '@/store';

interface DrilldownNavigatorProps {
  onBack?: () => void;
  onHome?: () => void;
}

const DrilldownNavigator: React.FC<DrilldownNavigatorProps> = ({ onBack, onHome }) => {
  const { drilldownPath, clearDrilldownPath } = useAppStore();

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

  const breadcrumbItems = drilldownPath.map((pathItem, index) => {
    const [type, id] = pathItem.split(':');
    const typeName =
      type === 'namespace'
        ? '命名空间'
        : type === 'node'
          ? '节点'
          : type === 'workload'
            ? '工作负载'
            : 'Pod';
    return {
      title: `${typeName}: ${id}`,
      key: index,
    };
  });

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
      {drilldownPath.length > 0 && <Breadcrumb items={breadcrumbItems} />}
    </Space>
  );
};

export default DrilldownNavigator;
