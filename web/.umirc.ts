import { defineConfig } from 'umi';

export default defineConfig({
  // API 代理：将 /api 转发到后端 8080
  proxy: {
    '/api': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
  },
  // 根路径重定向到成本透视页，使用 ProLayout 包裹四模块导航
  routes: [
    {
      path: '/',
      component: '@/layouts/index',
      routes: [
        { path: '/', redirect: '/CostOverviewPage' },
        { path: '/CostOverviewPage', component: '@/pages/CostOverviewPage' },
        { path: '/DrilldownPage', component: '@/pages/DrilldownPage' },
        { path: '/SLODashboard', component: '@/pages/SLODashboard' },
        { path: '/ROIDashboard', component: '@/pages/ROIDashboard' },
      ],
    },
  ],
});
