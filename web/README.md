# Lighthouse Web Frontend

Lighthouse前端仪表板，用于成本优化和资源效率监控。

## 技术栈

- **框架**: React 18 + TypeScript
- **UI库**: Ant Design Pro + Ant Design 5
- **状态管理**: Zustand
- **图表库**: Recharts
- **路由**: React Router v6
- **HTTP客户端**: Axios
- **测试**: Jest + React Testing Library + Playwright
- **文档**: Storybook
- **构建工具**: Umi.js

## 项目结构

```
src/
├── app.tsx                 # 应用入口配置
├── types/                  # TypeScript类型定义
│   └── index.ts
├── services/               # API服务层
│   ├── api.ts             # Axios客户端封装
│   ├── costService.ts     # 成本相关API服务
│   └── mockApi.ts         # Mock数据服务
├── store/                  # 状态管理
│   └── index.ts           # Zustand store
├── components/             # 可复用组件
│   ├── StatusIndicator.tsx    # 状态指示器（红绿灯）
│   ├── EfficiencyChart.tsx    # 效率分环形图
│   ├── TrendChart.tsx         # 趋势图表
│   ├── DrilldownNavigator.tsx # 钻取导航
│   └── CostTable.tsx          # 成本透视表格
├── pages/                  # 页面组件
│   ├── CostOverviewPage.tsx   # 成本透视主页面
│   ├── DrilldownPage.tsx      # 四层钻取详情页面
│   ├── SLODashboard.tsx       # SLO健康监控页面
│   └── ROIDashboard.tsx       # ROI价值追踪页面
└── __tests__/              # 测试文件
```

## 核心功能

### 1. 全域成本透视
- 展示Total Billable Cost、Total Waste（可优化空间）、Global Efficiency、Domain Breakdown
- 遵循去攻击性文案原则，使用"可优化空间"代替"浪费"

### 2. 成本透视表
- 展示Namespace级别的成本、可优化空间、效率分
- 支持渐进式披露，默认折叠优化建议，点击展开

### 3. 四层钻取
- 支持从Namespace→Node→Workload→Pod的逐层钻取
- 钻取导航组件提供面包屑导航和返回功能

### 4. SLO健康监控
- 红绿灯状态面板展示服务健康状态
- 基于效率分自动判断健康状态（>70%健康，50-70%警告，<50%严重）

### 5. ROI价值追踪
- 趋势图表展示价值、成本、效率分的变化
- 支持多时间维度查看（最近30天、完整历史）

## 状态管理

使用Zustand实现全局状态管理，包含以下特性：

- 管理当前选中的Namespace/Node/Workload/Pod
- 处理加载状态和错误状态
- 支持数据缓存和持久化（localStorage）
- 支持Mock数据切换（开发阶段使用Mock数据，生产环境调用真实API）

## API集成

- 封装API调用服务，统一处理请求和响应
- 处理API错误和重试机制
- 实现数据转换和格式化
- 支持Mock数据切换，便于开发和测试

## 响应式设计

- 支持桌面、平板、移动端适配
- 使用Ant Design的响应式栅格系统
- 移动端优化的交互体验

## 开发指南

### 安装依赖

```bash
npm install
```

### 启动开发服务器

```bash
npm start
```

### 运行测试

```bash
# 组件单元测试
npm test

# E2E测试
npm run e2e

# 代码质量检查
npm run lint
npm run format:check
```

### Storybook文档

```bash
# 启动Storybook
npm run storybook

# 构建Storybook静态站点
npm run build-storybook
```

### 构建生产版本

```bash
npm run build
```

## 环境变量

- `API_BASE_URL`: API基础URL，默认为`/api`
- 开发环境默认使用Mock数据
- 生产环境需要配置真实的API地址

## 测试覆盖

- **组件单元测试**: Jest + React Testing Library
- **E2E测试**: Playwright（支持多浏览器和移动设备）
- **响应式设计测试**: 包含在E2E测试中
- **Storybook文档**: 组件交互式文档和演示

## 代码规范

- 遵循ESLint和Prettier代码规范
- TypeScript严格模式
- 组件Props类型安全
- 去攻击性文案原则
- 渐进式披露设计原则

## 路由配置

- `/` - 成本透视主页面
- `/drilldown` - 四层钻取详情页面（带查询参数）
- `/slo` - SLO健康监控页面  
- `/roi` - ROI价值追踪页面