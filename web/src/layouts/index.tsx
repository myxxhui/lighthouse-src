import React, { useState } from 'react';
import { Layout, Menu, Switch } from 'antd';
import { useLocation, useNavigate, Outlet } from 'umi';
import {
  FundOutlined,
  ClusterOutlined,
  DashboardOutlined,
  SafetyCertificateOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons';
import { useAppStore } from '@/store';

const { Content } = Layout;

const SIDER_WIDTH = 220;
const SIDER_COLLAPSED_WIDTH = 64;
// [Ref: 用户需求] 品牌 SVG 资产路径，与 public/logo.svg 对应，构建后通过 nginx 静态根访问
const BRAND_LOGO_SRC = '/logo.svg';

const menuItems = [
  { key: '/CostOverviewPage', icon: <FundOutlined />, label: '全域成本透视' },
  { key: '/DrilldownPage', icon: <ClusterOutlined />, label: '成本钻取' },
  { key: '/SLODashboard', icon: <DashboardOutlined />, label: 'SLO 红绿灯' },
  { key: '/PreventionPage', icon: <SafetyCertificateOutlined />, label: '智能预防' },
];

/** [Ref: 01_实践] 仅保留一个左侧功能展示栏：Logo、收起按钮、导航菜单、深色主题均在左侧，无顶端横条 */
const BasicLayout: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);
  const { theme, setTheme } = useAppStore();
  const selectedKey = location.pathname || '/CostOverviewPage';

  return (
    <Layout style={{ height: '100vh', overflow: 'hidden', flexDirection: 'row' }}>
      <aside
        className={`lighthouse-sider ${collapsed ? 'lighthouse-sider-collapsed' : ''}`}
        style={{
          flex: `0 0 ${collapsed ? SIDER_COLLAPSED_WIDTH : SIDER_WIDTH}px`,
          width: collapsed ? SIDER_COLLAPSED_WIDTH : SIDER_WIDTH,
          minWidth: collapsed ? SIDER_COLLAPSED_WIDTH : SIDER_WIDTH,
          maxWidth: collapsed ? SIDER_COLLAPSED_WIDTH : SIDER_WIDTH,
          borderRight: '1px solid rgba(5,5,5,0.06)',
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
        }}
      >
        <div
          className="lighthouse-sider-logo"
          style={{
            height: 48,
            flexShrink: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'space-between',
            gap: collapsed ? 6 : 0,
            paddingLeft: collapsed ? 0 : 12,
            paddingRight: collapsed ? 0 : 8,
          }}
        >
          {collapsed ? (
            /* 折叠态：仅显示 Logo 图标，居中，尺寸略大避免形变 */
            <img
              src={BRAND_LOGO_SRC}
              alt="Lighthouse"
              style={{ width: 24, height: 24, objectFit: 'contain', display: 'block' }}
            />
          ) : (
            /* 展开态：Logo + 品牌文字，基线对齐 */
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
              <img
                src={BRAND_LOGO_SRC}
                alt="Lighthouse Logo"
                style={{ width: 22, height: 22, objectFit: 'contain', flexShrink: 0 }}
              />
              <span style={{ fontWeight: 600, fontSize: 16, lineHeight: '22px' }}>Lighthouse</span>
            </span>
          )}
          <span
            onClick={() => setCollapsed(!collapsed)}
            role="button"
            tabIndex={0}
            onKeyDown={e => e.key === 'Enter' && setCollapsed(!collapsed)}
            style={{ cursor: 'pointer', padding: 4, fontSize: 16 }}
            aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          </span>
        </div>
        <Menu
          theme={theme === 'dark' ? 'dark' : 'light'}
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          className="lighthouse-sider-menu"
          style={{ borderRight: 0, overflow: 'hidden', flex: 1, minHeight: 0 }}
        />
        <div style={{ flexShrink: 0, padding: 12, borderTop: '1px solid rgba(5,5,5,0.06)' }}>
          {!collapsed && <span style={{ fontSize: 12, color: 'rgba(0,0,0,0.45)', marginRight: 8 }}>深色主题</span>}
          <Switch
            size="small"
            checked={theme === 'dark'}
            onChange={on => setTheme(on ? 'dark' : 'light')}
            checkedChildren="开"
            unCheckedChildren="关"
          />
        </div>
      </aside>
      <Content style={{ flex: 1, minHeight: 0, padding: 24, overflow: 'auto' }}>
        <Outlet />
      </Content>
    </Layout>
  );
};

export default BasicLayout;
