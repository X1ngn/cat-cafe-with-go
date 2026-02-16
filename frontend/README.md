# 猫猫咖啡屋 - 前端应用

基于 Figma 设计的多 Agent 协作聊天应用前端实现。

## 技术栈

- **React 18** - UI 框架
- **TypeScript 5** - 类型安全
- **Tailwind CSS 3** - 样式框架
- **Zustand 4** - 状态管理
- **Axios** - HTTP 客户端
- **Vite 5** - 构建工具

## 快速开始

### 安装依赖

```bash
npm install
```

### 启动开发服务器

```bash
npm run dev
```

访问 http://localhost:3000

### 构建生产版本

```bash
npm run build
```

### 预览生产版本

```bash
npm run preview
```

## 项目结构

```
frontend/
├── docs/                    # 文档
│   ├── API.md              # API 接口文档
│   └── DESIGN.md           # 设计文档
├── src/
│   ├── components/         # React 组件
│   │   ├── Sidebar/       # 左侧会话列表
│   │   ├── ChatArea/      # 中间对话区
│   │   ├── StatusBar/     # 右侧状态栏
│   │   └── common/        # 通用组件
│   ├── services/          # API 服务
│   ├── stores/            # Zustand 状态管理
│   ├── types/             # TypeScript 类型定义
│   └── styles/            # 全局样式
├── package.json
├── tsconfig.json
├── vite.config.ts
└── tailwind.config.js
```

## 核心功能

### 1. 会话管理
- 创建新会话
- 查看会话列表
- 切换会话
- 显示会话摘要和更新时间

### 2. 消息交互
- 发送消息
- 查看历史消息
- @ 提及猫猫 Agent
- 区分猫猫消息、用户消息、系统消息

### 3. @ 提及功能
- 输入 `@` 弹出猫猫选择菜单
- 显示猫猫状态（待命/工作中）
- 支持搜索过滤
- 多猫猫协作

### 4. 状态监控
- 实时显示猫猫状态
- 消息统计（总消息数、猫猫消息数）
- 调用历史记录

## 设计还原

本项目完全基于 Figma 设计稿实现，包括：

- **三栏布局**: 左侧会话列表（280px）+ 中间对话区（自适应）+ 右侧状态栏（480px）
- **颜色系统**: 提取 Figma 设计中的所有颜色值
- **组件样式**: 1:1 还原消息气泡、头像、状态标签等
- **交互细节**: 悬停效果、选中状态、打字指示器

## API 对接

后端需要实现以下 API 接口（详见 `docs/API.md`）：

### 会话管理
- `GET /api/sessions` - 获取会话列表
- `POST /api/sessions` - 创建新会话
- `GET /api/sessions/:id` - 获取会话详情
- `DELETE /api/sessions/:id` - 删除会话

### 消息管理
- `GET /api/sessions/:id/messages` - 获取消息列表
- `POST /api/sessions/:id/messages` - 发送消息
- `GET /api/sessions/:id/stats` - 获取消息统计

### 猫猫管理
- `GET /api/cats` - 获取所有猫猫
- `GET /api/cats/:id` - 获取猫猫状态
- `GET /api/cats/available` - 获取可用猫猫

### 调用历史
- `GET /api/sessions/:id/history` - 获取调用历史

## 配置

### API 代理

开发环境下，API 请求会自动代理到 `http://localhost:8080`。

修改 `vite.config.ts` 中的 proxy 配置：

```typescript
server: {
  proxy: {
    '/api': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
  },
}
```

### 环境变量

创建 `.env` 文件：

```
VITE_API_BASE_URL=http://localhost:8080/api
```

## 开发指南

### 添加新组件

1. 在 `src/components/` 下创建组件目录
2. 创建 `index.tsx` 和相关子组件
3. 导出组件供其他模块使用

### 添加新 API

1. 在 `src/services/api.ts` 中添加 API 方法
2. 在 `src/types/index.ts` 中定义类型
3. 在组件中调用 API

### 状态管理

使用 Zustand 进行全局状态管理：

```typescript
import { useAppStore } from '@/stores/appStore';

function MyComponent() {
  const { currentSession, setCurrentSession } = useAppStore();
  // ...
}
```

## 样式开发

使用 Tailwind CSS 工具类：

```tsx
<div className="bg-white rounded-lg p-4 shadow-md">
  {/* 内容 */}
</div>
```

自定义颜色在 `tailwind.config.js` 中定义：

```javascript
colors: {
  primary: '#ccd9e5',
  'cat-orange': '#ff9966',
  // ...
}
```

## 浏览器支持

- Chrome >= 90
- Firefox >= 88
- Safari >= 14
- Edge >= 90

## 文档

- [API 接口文档](./docs/API.md)
- [设计文档](./docs/DESIGN.md)

## 许可证

MIT
