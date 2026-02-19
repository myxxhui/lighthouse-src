import React from 'react';
import { ProLayout } from '@ant-design/pro-layout';
import { useLocation, useNavigate, Outlet } from 'umi';
import { FundOutlined, ClusterOutlined, DashboardOutlined, LineChartOutlined } from '@ant-design/icons';

const defaultProps = {
  route: {
    path: '/',
    routes: [
      { path: '/CostOverviewPage', name: '全域成本透视', icon: <FundOutlined /> },
      { path: '/DrilldownPage', name: '四层钻取', icon: <ClusterOutlined /> },
      { path: '/SLODashboard', name: 'SLO 红绿灯', icon: <DashboardOutlined /> },
      { path: '/ROIDashboard', name: 'ROI 看板', icon: <LineChartOutlined /> },
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
