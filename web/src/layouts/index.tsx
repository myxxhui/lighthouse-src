import React from 'react';
import { ProLayout } from '@ant-design/pro-layout';
import { useLocation, useNavigate, Outlet } from 'umi';
import {
  FundOutlined,
  ClusterOutlined,
  DashboardOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
} from '@ant-design/icons';

const defaultProps = {
  route: {
    path: '/',
    routes: [
      { path: '/CostOverviewPage', name: '全域成本透视', icon: <FundOutlined /> },
      { path: '/DrilldownPage', name: '成本钻取', icon: <ClusterOutlined /> },
      { path: '/SLODashboard', name: 'SLO 红绿灯', icon: <DashboardOutlined /> },
      { path: '/PreventionPage', name: '智能预防', icon: <SafetyCertificateOutlined /> },
      { path: '/FaultHandlingPage', name: '智能故障处理', icon: <ToolOutlined /> },
    ],
  },
  location: { pathname: '/' },
};

const BasicLayout: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();

  return (
    <ProLayout
      title="Lighthouse"
      {...defaultProps}
      location={location}
      menuItemRender={(item, dom) => (
        <div
          onClick={() => {
            navigate(item.path || '/');
          }}
        >
          {dom}
        </div>
      )}
    >
      <div style={{ padding: 24 }}>
        <Outlet />
      </div>
    </ProLayout>
  );
};

export default BasicLayout;
