import { defineConfig } from 'umi';

export default defineConfig({
  define: {
    // 构建时注入：与后端 FINOPS_SYNC_JOB_API_KEY 一致时，浏览器 POST /finops/sync-jobs 可带 X-FinOps-Sync-Key（公网建议改由网关注入，勿把密钥提交仓库）
    'process.env.FINOPS_SYNC_JOB_KEY': process.env.FINOPS_SYNC_JOB_KEY || '',
  },
  // 解决 esbuild helpers 冲突（见 build 报错 esbuildHelperChecker）
  esbuildMinifyIIFE: true,
  // [Ref: 用户需求] 品牌 Favicon 与页面标题，Umi 4 将在 index.html <head> 注入 <link rel="icon"> 标签
  title: 'Lighthouse',
  favicons: ['/logo.svg'],
  // API 代理：将 /api 转发到后端 8080
  proxy: {
    '/api': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
  },
  // 扁平路由：Umi 约定式布局自动发现 src/layouts/index.tsx 作为全局唯一侧栏布局，
  // 无需在此显式包裹 component: '@/layouts/index'，避免双重渲染
  routes: [
    { path: '/', redirect: '/CostOverviewPage' },
    { path: '/CostOverviewPage', component: '@/pages/CostOverviewPage' },
    { path: '/DrilldownPage', component: '@/pages/DrilldownPage' },
    { path: '/CostDrilldownEnvPage', component: '@/pages/CostDrilldownEnvPage' },
    { path: '/SLODashboard', component: '@/pages/SLODashboard' },
    { path: '/PreventionPage', component: '@/pages/PreventionPage' },
    { path: '/FaultHandlingPage', component: '@/pages/FaultHandlingPage' },
  ],
});
