import React from 'react';
import { Card, Alert } from 'antd';
import { ToolOutlined } from '@ant-design/icons';

const FaultHandlingPage: React.FC = () => {
  return (
    <div>
      <h2>
        <ToolOutlined style={{ marginRight: 8 }} />
        智能故障处理
      </h2>
      <Alert
        message="即将推出"
        description="本能力规划中，敬请期待。"
        type="info"
        showIcon
        style={{ marginBottom: 24 }}
      />
      <Card title="能力价值">
        <p>
          <strong>智能故障处理</strong>：自动追踪故障链路，精准根因查询，一键授权修复。
        </p>
        <p style={{ color: '#666', marginTop: 8 }}>
          当 SLO 透视检测到故障时，自动完成链路追踪与根因分析，支持一键修复，降低 MTTR，提升排查效率。
        </p>
        <p style={{ color: '#999', fontSize: 12, marginTop: 16 }}>
          了解能力规划请参阅项目文档「顶层概念总览」。
        </p>
      </Card>
    </div>
  );
};

export default FaultHandlingPage;
