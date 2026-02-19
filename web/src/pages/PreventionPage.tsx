import React from 'react';
import { Card, Alert } from 'antd';
import { SafetyCertificateOutlined } from '@ant-design/icons';

const PreventionPage: React.FC = () => {
  return (
    <div>
      <h2>
        <SafetyCertificateOutlined style={{ marginRight: 8 }} />
        智能预防
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
          <strong>智能预防</strong>：提前预知风险，自动报告问题，提供解决方案并支持一键授权修复。
        </p>
        <p style={{ color: '#666', marginTop: 8 }}>
          基于历史数据与趋势分析，在成本透视与 SLO 透视基础上识别潜在风险，减少故障发生，提升系统稳定性。
        </p>
        <p style={{ color: '#999', fontSize: 12, marginTop: 16 }}>
          了解能力规划请参阅项目文档「顶层概念总览」。
        </p>
      </Card>
    </div>
  );
};

export default PreventionPage;
