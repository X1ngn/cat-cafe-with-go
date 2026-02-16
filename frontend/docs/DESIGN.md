# 猫猫咖啡屋前端设计文档

## 项目概述

猫猫咖啡屋是一个多 Agent 协作的聊天应用，用户可以通过 @ 提及的方式呼叫不同的猫猫 Agent 来协助完成任务。前端采用 React + TypeScript + Tailwind CSS 技术栈。

## 技术栈

- **框架**: React 18
- **语言**: TypeScript 5
- **样式**: Tailwind CSS 3
- **状态管理**: Zustand 4
- **HTTP 客户端**: Axios 1.6
- **构建工具**: Vite 5

## 项目结构

```
frontend/
├── docs/                    # 文档目录
│   ├── API.md              # API 接口文档
│   └── DESIGN.md           # 设计文档（本文件）
├── public/                  # 静态资源
├── src/
│   ├── components/         # 组件目录
│   │   ├── Sidebar/       # 左侧边栏
│   │   │   ├── index.tsx
│   │   │   └── SessionCard.tsx
│   │   ├── ChatArea/      # 中间对话区
│   │   │   ├── index.tsx
│   │   │   ├── MessageBubble.tsx
│   │   │   ├── MessageInput.tsx
│   │   │   └── MentionMenu.tsx
│   │   ├── StatusBar/     # 右侧状态栏
│   │   │   └── index.tsx
│   │   └── common/        # 通用组件
│   │       ├── Avatar.tsx
│   │       ├── Button.tsx
│   │       └── StatusBadge.tsx
│   ├── hooks/             # 自定义 Hooks
│   ├── services/          # API 服务
│   │   └── api.ts
│   ├── stores/            # 状态管理
│   │   └── appStore.ts
│   ├── styles/            # 样式文件
│   │   └── index.css
│   ├── types/             # 类型定义
│   │   └── index.ts
│   ├── utils/             # 工具函数
│   ├── App.tsx            # 主应用组件
│   └── main.tsx           # 入口文件
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
├── tailwind.config.js
└── postcss.config.js
```

## 设计规范

### 颜色系统

根据 Figma 设计提取的颜色：

```javascript
colors: {
  primary: '#ccd9e5',           // 主色调（按钮等）
  'cat-orange': '#ff9966',      // 猫猫头像-橘色
  'cat-beige': '#d9bf99',       // 猫猫头像-米色
  'user-green': '#d9f2e5',      // 用户消息气泡
  'bg-cream': '#faf5f0',        // 背景色
  'status-idle': '#ccf2cc',     // 待命状态
  'status-busy': '#ffd9cc',     // 工作中状态
}
```

### 布局规范

- **左侧边栏**: 280px 固定宽度
- **右侧状态栏**: 480px 固定宽度
- **中间对话区**: 自适应宽度
- **总宽度**: 1920px（设计稿）
- **总高度**: 1080px（设计稿）

### 组件规范

#### 头像 (Avatar)
- **小号 (sm)**: 32x32px, 圆角 16px
- **中号 (md)**: 48x48px, 圆角 24px
- **大号 (lg)**: 64x64px, 圆角 32px

#### 消息气泡
- **猫猫消息**: 灰色背景 (#f2f2f2)，左对齐
- **用户消息**: 绿色背景 (#d9f2e5)，右对齐
- **系统消息**: 灰色背景 (#e6e6e6)，居中，小字号

#### 状态标签
- **待命**: 绿色背景 (#ccf2cc)，绿色文字 (#669966)
- **工作中**: 橙色背景 (#ffd9cc)，橙色文字 (#cc8c66)
- **离线**: 灰色背景 (#cccccc)，灰色文字 (#666666)

## 核心功能

### 1. 会话管理

**功能描述**:
- 显示会话列表
- 创建新会话
- 切换会话
- 删除会话

**实现要点**:
- 会话列表按更新时间倒序排列
- 当前选中会话高亮显示
- 显示会话名称、更新时间、消息摘要

### 2. 消息展示

**功能描述**:
- 显示消息列表
- 区分猫猫消息、用户消息、系统消息
- 自动滚动到最新消息

**实现要点**:
- 消息按时间顺序排列
- 猫猫消息显示头像和名称
- 用户消息右对齐
- 系统消息居中显示

### 3. @ 提及功能

**功能描述**:
- 输入 @ 符号时弹出猫猫选择菜单
- 显示猫猫名称、头像、状态
- 支持搜索过滤
- 选择后插入猫猫名称

**实现要点**:
- 监听输入框的 @ 符号
- 提取 @ 后的查询字符串进行过滤
- 点击选择后替换 @ 及查询字符串
- 记录被提及的猫猫 ID 列表

### 4. 状态展示

**功能描述**:
- 显示所有猫猫的实时状态
- 显示消息统计数据
- 显示调用历史记录

**实现要点**:
- 定期轮询或通过 WebSocket 更新状态
- 统计数据实时更新
- 历史记录按时间倒序排列

## 状态管理

使用 Zustand 进行全局状态管理：

```typescript
interface AppState {
  // 当前会话
  currentSession: Session | null;

  // 会话列表
  sessions: Session[];

  // 消息列表
  messages: Message[];

  // 猫猫列表
  cats: Cat[];

  // 输入框内容
  inputValue: string;

  // Mention 菜单状态
  showMentionMenu: boolean;
  mentionQuery: string;
}
```

## API 集成

### 服务层设计

所有 API 调用封装在 `services/api.ts` 中：

- `sessionAPI`: 会话相关接口
- `messageAPI`: 消息相关接口
- `catAPI`: 猫猫相关接口
- `historyAPI`: 历史记录接口

### 错误处理

- 使用 Axios 拦截器统一处理错误
- 显示用户友好的错误提示
- 记录错误日志

## 性能优化

### 1. 消息列表优化
- 使用虚拟滚动处理大量消息
- 分页加载历史消息
- 消息去重

### 2. 状态更新优化
- 使用 WebSocket 替代轮询（推荐）
- 轮询时使用合理的间隔（5-10秒）
- 仅在会话活跃时更新

### 3. 组件优化
- 使用 React.memo 避免不必要的重渲染
- 合理使用 useMemo 和 useCallback
- 懒加载非关键组件

## 响应式设计

虽然设计稿为 1920x1080，但应考虑不同屏幕尺寸：

- **大屏 (>1920px)**: 保持固定宽度，居中显示
- **中屏 (1280-1920px)**: 自适应宽度
- **小屏 (<1280px)**: 考虑隐藏右侧状态栏或使用抽屉式设计

## 可访问性

- 使用语义化 HTML 标签
- 提供键盘导航支持
- 添加 ARIA 标签
- 确保颜色对比度符合 WCAG 标准

## 测试策略

### 单元测试
- 测试工具函数
- 测试状态管理逻辑
- 测试 API 服务层

### 组件测试
- 测试组件渲染
- 测试用户交互
- 测试边界情况

### E2E 测试
- 测试完整的用户流程
- 测试跨组件交互

## 部署

### 构建
```bash
npm run build
```

### 预览
```bash
npm run preview
```

### 环境变量
- `VITE_API_BASE_URL`: API 基础地址
- `VITE_WS_URL`: WebSocket 地址

## 未来扩展

### 1. 实时通信
- 实现 WebSocket 连接
- 实时消息推送
- 打字状态指示器

### 2. 富文本支持
- Markdown 渲染
- 代码高亮
- 图片上传

### 3. 用户设置
- 主题切换
- 字体大小调整
- 通知设置

### 4. 搜索功能
- 全局消息搜索
- 会话搜索
- 猫猫搜索

### 5. 导出功能
- 导出会话记录
- 导出为 PDF/Markdown

## 开发指南

### 安装依赖
```bash
cd frontend
npm install
```

### 启动开发服务器
```bash
npm run dev
```

### 代码规范
- 使用 ESLint 进行代码检查
- 使用 Prettier 进行代码格式化
- 遵循 TypeScript 严格模式

### Git 提交规范
- `feat`: 新功能
- `fix`: 修复 bug
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具相关

## 常见问题

### Q: 如何添加新的猫猫？
A: 后端添加猫猫数据后，前端会自动通过 API 获取并显示。

### Q: 如何自定义主题颜色？
A: 修改 `tailwind.config.js` 中的 `colors` 配置。

### Q: 如何处理长消息？
A: 消息气泡会自动换行，最大宽度为 400px（max-w-md）。

### Q: 如何实现消息撤回？
A: 需要后端支持，前端调用删除消息 API 并从列表中移除。

## 联系方式

如有问题，请联系开发团队或提交 Issue。
