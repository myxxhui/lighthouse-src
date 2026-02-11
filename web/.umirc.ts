import { defineConfig } from 'umi';

export default defineConfig({
  routes: [
    { path: '/', component: '@/pages/CostOverviewPage' },
    { path: '/drilldown', component: '@/pages/DrilldownPage' },
    { path: '/slo', component: '@/pages/SLODashboard' },
    { path: '/roi', component: '@/pages/ROIDashboard' },
  ],
  fastRefresh: true,
  proxy: {
    '/api': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
  },
});