# 可访问性架构设计文档

## 【需求抽象】

**工程问题：** 实现符合 WCAG 2.1 AA 标准的可访问性组件库

**核心目标：**
1. 修复严重问题：焦点陷阱、表单错误处理
2. 修复中等问题：触控热区、对比度、图片语义化
3. 实现建议项：读屏播报系统

**技术约束：**
- 必须兼容现有 React + TypeScript 技术栈
- 必须支持 Tailwind CSS
- 必须零依赖或最小依赖

---

## 【系统设计】

### 架构分层

```
┌─────────────────────────────────────┐
│      应用层 (Application)           │
│  使用可访问性组件构建业务功能        │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│      组件层 (Components)            │
│  Modal, Drawer, Form, Input, etc.   │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│      Hooks 层 (Hooks)               │
│  useFocusTrap, useAnnouncer         │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│      工具层 (Utils)                 │
│  a11y.ts - 辅助函数                 │
└─────────────────────────────────────┘
```

### 模块职责

**1. Hooks 层**
- `useFocusTrap` - 焦点陷阱管理
- `useAnnouncer` - 读屏播报

**2. Components 层**
- `Modal` - 模态框（带焦点陷阱）
- `Drawer` - 抽屉（带焦点陷阱）
- `Form` - 表单（自动错误聚焦）
- `Input` - 输入框（ARIA 关联）
- `IconButton` - 图标按钮（触控优化）
- `AccessibleImage` - 图片（语义化）
- `AnnouncerProvider` - 全局播报器

**3. Utils 层**
- ID 生成
- 焦点元素查询
- 对比度检查
- 常量定义

---

## 【接口定义】

### 核心接口

```typescript
// 焦点陷阱配置
interface FocusTrapOptions {
  initialFocus?: RefObject<HTMLElement>;
  returnFocus?: RefObject<HTMLElement>;
  onEscape?: () => void;
  isActive: boolean;
}

// 播报器接口
interface Announcer {
  announce: (message: string, priority?: 'polite' | 'assertive') => void;
}

// 可访问覆盖层通用接口
interface AccessibleOverlayProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  children: ReactNode;
  initialFocusRef?: RefObject<HTMLElement>;
  returnFocusRef?: RefObject<HTMLElement>;
  'aria-labelledby'?: string;
  'aria-describedby'?: string;
}

// 可访问输入框接口
interface AccessibleInputProps {
  label: string;
  error?: string;
  required?: boolean;
  helperText?: string;
}
```

---

## 【核心实现】

### 目录结构

```
frontend/src/
├── hooks/
│   ├── useFocusTrap.ts          # 焦点陷阱 Hook
│   └── useAnnouncer.ts          # 读屏播报 Hook
├── components/ui/
│   ├── Modal.tsx                # 模态框组件
│   ├── Drawer.tsx               # 抽屉组件
│   ├── Form.tsx                 # 表单组件
│   ├── Input.tsx                # 输入框组件
│   ├── IconButton.tsx           # 图标按钮组件
│   ├── AccessibleImage.tsx      # 图片组件
│   └── AnnouncerProvider.tsx    # 播报器 Provider
├── utils/
│   └── a11y.ts                  # 可访问性工具函数
├── a11y.ts                      # 统一导出
└── docs/
    └── accessibility-guide.md   # 使用指南
```

### 关键技术实现

**1. 焦点陷阱机制**
```typescript
// 核心逻辑：
// 1. 打开时保存当前焦点
// 2. 将焦点移到容器内第一个可聚焦元素
// 3. 监听 Tab 键，实现循环聚焦
// 4. 监听 Esc 键，触发关闭
// 5. 关闭时恢复原焦点
```

**2. 表单错误聚焦**
```typescript
// 核心逻辑：
// 1. 监听 errors 对象变化
// 2. 检测是否有新错误产生
// 3. 查找第一个错误字段的输入框
// 4. 聚焦并滚动到视图中
```

**3. ARIA Live Region**
```typescript
// 核心逻辑：
// 1. 创建两个隐藏的 div（polite 和 assertive）
// 2. 设置 aria-live 属性
// 3. 通过修改 textContent 触发播报
// 4. 使用 setTimeout 确保读屏软件检测到变化
```

---

## 【待薇薇审核项】

@薇薇 请审查以下实现：

1. **焦点陷阱实现** (`useFocusTrap.ts`)
   - 是否正确处理所有边界情况？
   - 是否有内存泄漏风险？

2. **ARIA 属性使用** (所有组件)
   - `aria-labelledby`, `aria-describedby`, `aria-invalid` 等是否正确？
   - 是否符合 ARIA 最佳实践？

3. **表单错误处理** (`Form.tsx`)
   - 自动聚焦逻辑是否可能导致用户体验问题？
   - 是否需要添加防抖？

4. **读屏播报时机** (`useAnnouncer.ts`)
   - 100ms 延迟是否合适？
   - 是否需要支持播报队列？

---

## 【待小乔设计项】

@小乔 请设计以下视觉规范：

1. **焦点指示器样式**
   - 当前使用 `focus:ring-2 focus:ring-blue-500`
   - 是否需要更明显的焦点样式？
   - 是否需要支持暗色模式？

2. **错误状态视觉**
   - 当前错误输入框使用红色边框
   - 是否需要添加错误图标？
   - 错误文本颜色是否足够明显？

3. **触控热区视觉反馈**
   - 当前使用 `hover:bg-gray-100`
   - 移动端是否需要 active 状态？
   - 是否需要涟漪效果？

4. **模态框/抽屉动画**
   - 当前缺少进入/退出动画
   - 建议添加淡入淡出效果
   - 抽屉需要滑入滑出动画

---

## 【技术决策记录】

### 为什么不使用第三方库？

**考虑过的方案：**
- `react-focus-lock` - 功能强大但体积较大（~10KB）
- `@radix-ui/react-dialog` - 完整但引入整个 UI 库
- `focus-trap-react` - 专注但仍需额外依赖

**最终决策：** 自研实现
- 零依赖，完全可控
- 代码量小（< 200 行）
- 可根据项目需求定制

### 为什么使用 Context 而不是单例？

**AnnouncerProvider 使用 Context 的原因：**
- 支持多个独立的 React 应用实例
- 便于测试（可以 mock Provider）
- 符合 React 生态习惯

### 为什么不使用 React Hook Form？

**当前 Form 组件是轻量级实现：**
- 仅处理错误聚焦，不管理表单状态
- 可以与任何表单库配合使用
- 如果需要完整表单方案，建议集成 React Hook Form

---

## 【性能考虑】

1. **焦点陷阱性能**
   - 使用事件委托，只在容器上监听
   - 懒查询可聚焦元素（按需计算）

2. **ARIA Live Region**
   - 全局只创建两个 div
   - 使用 setTimeout 批处理播报

3. **组件渲染优化**
   - 使用 `React.memo` 包裹纯展示组件
   - 避免在 render 中创建新对象

---

## 【未来扩展】

1. **国际化支持**
   - 错误提示文本支持多语言
   - 播报消息支持多语言

2. **主题系统**
   - 支持自定义焦点颜色
   - 支持暗色模式

3. **更多组件**
   - Select（下拉选择）
   - Checkbox/Radio（复选框/单选框）
   - Tooltip（工具提示）
   - Toast（通知）

---

喵~ 这是一个可以活很多年的可访问性系统！🐱✨
