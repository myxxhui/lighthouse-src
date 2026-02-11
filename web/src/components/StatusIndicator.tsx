import React from 'react';
import { Badge, Tooltip } from 'antd';

interface StatusIndicatorProps {
  status: 'healthy' | 'warning' | 'critical';
  size?: 'small' | 'default' | 'large';
  showText?: boolean;
}

const StatusIndicator: React.FC<StatusIndicatorProps> = ({
  status,
  size = 'default',
  showText = false,
}) => {
  const getStatusColor = () => {
    switch (status) {
      case 'healthy':
        return '#52c41a'; // 绿色
      case 'warning':
        return '#faad14'; // 黄色
      case 'critical':
        return '#ff4d4f'; // 红色
      default:
        return '#d9d9d9'; // 灰色
    }
  };

  const getStatusText = () => {
    switch (status) {
      case 'healthy':
        return '健康';
      case 'warning':
        return '警告';
      case 'critical':
        return '严重';
      default:
        return '未知';
    }
  };

  const badgeSize = size === 'small' ? 8 : size === 'large' ? 16 : 12;

  return (
    <Tooltip title={getStatusText()}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
        <Badge
          data-testid="status-indicator"
          color={getStatusColor()}
          style={{ width: badgeSize, height: badgeSize, borderRadius: '50%' }}
        />
        {showText && <span>{getStatusText()}</span>}
      </div>
    </Tooltip>
  );
};

export default StatusIndicator;
