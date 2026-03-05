import React, { useState } from 'react';
import { Layout, Menu, Switch } from 'antd';
import { useLocation, useNavigate, Outlet } from 'umi';
import {
  FundOutlined,
  ClusterOutlined,
  DashboardOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons';
import { useAppStore } from '@/store';

const { Content } = Layout;

const SIDER_WIDTH = 220;
const SIDER_COLLAPSED_WIDTH = 64;

// [Ref: 用户需求] 内联 SVG 组件——消除 <img src> 的路径/加载/尺寸推算问题
const LighthouseLogo: React.FC<{ size?: number }> = ({ size = 32 }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 32 32"
    xmlns="http://www.w3.org/2000/svg"
    fill="none"
    style={{ display: 'block', flexShrink: 0 }}
  >
    <defs>
      <linearGradient id="lhg" x1="16" y1="0" x2="16" y2="32" gradientUnits="userSpaceOnUse">
        <stop offset="0%" stopColor="#00BFFF" />
        <stop offset="100%" stopColor="#0047AB" />
      </linearGradient>
      <linearGradient id="lhgfill" x1="16" y1="2" x2="16" y2="15" gradientUnits="userSpaceOnUse">
        <stop offset="0%" stopColor="#00BFFF" stopOpacity={0.22} />
        <stop offset="100%" stopColor="#0047AB" stopOpacity={0.06} />
      </linearGradient>
    </defs>
    <line x1="16" y1="1.5" x2="16" y2="0.2" stroke="url(#lhg)" strokeWidth="1.3" strokeLinecap="round" />
    <line x1="16" y1="3" x2="19.4" y2="1.2" stroke="url(#lhg)" strokeWidth="0.95" strokeLinecap="round" opacity="0.65" />
    <line x1="16" y1="3" x2="12.6" y2="1.2" stroke="url(#lhg)" strokeWidth="0.95" strokeLinecap="round" opacity="0.65" />
    <polygon points="16,2.5 22.5,8.5 16,14.5 9.5,8.5" fill="url(#lhgfill)" />
    <polygon points="16,2.5 22.5,8.5 16,14.5 9.5,8.5" stroke="url(#lhg)" strokeWidth="1.5" strokeLinejoin="round" />
    <line x1="11" y1="12.5" x2="5" y2="29.5" stroke="url(#lhg)" strokeWidth="1.5" strokeLinecap="round" />
    <line x1="21" y1="12.5" x2="27" y2="29.5" stroke="url(#lhg)" strokeWidth="1.5" strokeLinecap="round" />
    <line x1="5" y1="29.5" x2="27" y2="29.5" stroke="url(#lhg)" strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

const menuItems = [
  { key: '/CostOverviewPage', icon: <FundOutlined />, label: '全域成本透视' },
  { key: '/DrilldownPage', icon: <ClusterOutlined />, label: '成本钻取' },
  { key: '/SLODashboard', icon: <DashboardOutlined />, label: 'SLO 红绿灯' },
  { key: '/PreventionPage', icon: <SafetyCertificateOutlined />, label: '智能预防' },
  { key: '/FaultHandlingPage', icon: <ToolOutlined />, label: '智能故障处理' },
];

/** [Ref: 01_实践] 仅保留一个左侧功能展示栏：Logo、收起按钮、导航菜单、深色主题均在左侧，无顶端横条 */
const BasicLayout: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);
  const { theme, setTheme } = useAppStore();
  const isDark = theme === 'dark';
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
          background: isDark ? '#141414' : '#ffffff',
          borderRight: `1px solid ${isDark ? 'rgba(255,255,255,0.06)' : 'rgba(5,5,5,0.06)'}`,
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          transition: 'width 0.2s, min-width 0.2s, max-width 0.2s, flex 0.2s, background 0.2s',
        }}
      >
        {/* Logo 区域 */}
        <div
          className="lighthouse-sider-logo"
          style={{
            height: 56,
            flexShrink: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'flex-start',
            paddingLeft: collapsed ? 0 : 16,
            paddingRight: collapsed ? 0 : 10,
            gap: collapsed ? 0 : 10,
          }}
        >
          <LighthouseLogo size={collapsed ? 28 : 30} />
          {!collapsed && (
            <span style={{
              fontWeight: 700,
              fontSize: 17,
              letterSpacing: '-0.02em',
              color: isDark ? 'rgba(255,255,255,0.88)' : 'rgba(0,0,0,0.85)',
              whiteSpace: 'nowrap',
              lineHeight: '30px',
            }}>
              Lighthouse
            </span>
          )}
          {/* 折叠按钮推到最右 */}
          <span
            onClick={() => setCollapsed(!collapsed)}
            role="button"
            tabIndex={0}
            onKeyDown={e => e.key === 'Enter' && setCollapsed(!collapsed)}
            style={{
              cursor: 'pointer',
              padding: 4,
              fontSize: 14,
              color: isDark ? 'rgba(255,255,255,0.45)' : 'rgba(0,0,0,0.45)',
              marginLeft: collapsed ? 0 : 'auto',
              flexShrink: 0,
            }}
            aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          </span>
        </div>

        <Menu
          theme={isDark ? 'dark' : 'light'}
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          className="lighthouse-sider-menu"
          style={{ borderRight: 0, overflow: 'hidden', flex: 1, minHeight: 0 }}
        />

        <div style={{
          flexShrink: 0,
          padding: 12,
          borderTop: `1px solid ${isDark ? 'rgba(255,255,255,0.06)' : 'rgba(5,5,5,0.06)'}`,
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}>
          {!collapsed && (
            <span style={{
              fontSize: 12,
              color: isDark ? 'rgba(255,255,255,0.38)' : 'rgba(0,0,0,0.45)',
            }}>
              深色主题
            </span>
          )}
          <Switch
            size="small"
            checked={isDark}
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
