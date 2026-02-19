# SessionNotFound 组件使用指南

## 📋 组件概述

`SessionNotFound` 是一个友好的错误状态页面组件，用于处理用户访问不存在的会话时的情况。

**特性：**
- ✅ 支持多种错误类型（不存在、权限不足、已删除）
- ✅ 可自定义错误消息
- ✅ 符合可访问性标准（ARIA 标签、键盘导航）
- ✅ 响应式设计
- ✅ 猫咪咖啡屋品牌风格

---

## 🎯 使用场景

1. **会话不存在** - 用户访问的会话 ID 不存在
2. **权限不足** - 用户无权访问该会话
3. **会话已删除** - 会话已被删除
4. **URL 错误** - 会话 ID 格式错误

---

## 📦 组件接口

```typescript
interface SessionNotFoundProps {
  /** 错误类型 */
  errorType?: 'not-found' | 'permission-denied' | 'deleted';
  /** 自定义错误消息 */
  message?: string;
  /** 返回首页回调 */
  onGoHome?: () => void;
  /** 创建新会话回调 */
  onCreateNew?: () => void;
}
```

---

## 🚀 基础用法

### 1. 在路由中使用

```tsx
import { SessionNotFound } from '@/components/common/SessionNotFound';
import { useNavigate } from 'react-router-dom';

function SessionPage() {
  const navigate = useNavigate();
  const { sessionId } = useParams();
  const [session, setSession] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // 加载会话数据
    fetchSession(sessionId)
      .then(setSession)
      .catch(() => setSession(null))
      .finally(() => setLoading(false));
  }, [sessionId]);

  if (loading) {
    return <LoadingSpinner />;
  }

  if (!session) {
    return (
      <SessionNotFound
        errorType="not-found"
        onGoHome={() => navigate('/')}
        onCreateNew={() => navigate('/new-session')}
      />
    );
  }

  return <SessionContent session={session} />;
}
```

### 2. 处理权限错误

```tsx
function ProtectedSessionPage() {
  const navigate = useNavigate();
  const { session, hasPermission } = useSession();

  if (!session) {
    return (
      <SessionNotFound
        errorType="not-found"
        onGoHome={() => navigate('/')}
      />
    );
  }

  if (!hasPermission) {
    return (
      <SessionNotFound
        errorType="permission-denied"
        onGoHome={() => navigate('/')}
      />
    );
  }

  return <SessionContent session={session} />;
}
```

### 3. 处理已删除的会话

```tsx
function SessionPage() {
  const navigate = useNavigate();
  const { session, isDeleted } = useSession();

  if (isDeleted) {
    return (
      <SessionNotFound
        errorType="deleted"
        onGoHome={() => navigate('/')}
        onCreateNew={() => navigate('/new-session')}
      />
    );
  }

  return <SessionContent session={session} />;
}
```

### 4. 自定义错误消息

```tsx
<SessionNotFound
  errorType="not-found"
  message="这个会话可能已经过期了喵，请创建一个新的会话继续对话。"
  onGoHome={() => navigate('/')}
  onCreateNew={() => navigate('/new-session')}
/>
```

---

## 🎨 错误类型说明

### not-found（默认）
- **标题：** 会话不存在
- **描述：** 抱歉喵，找不到这个会话。可能是链接有误，或者会话已经被删除了。
- **图标：** 🔍

### permission-denied
- **标题：** 无法访问此会话
- **描述：** 抱歉喵，您没有权限访问这个会话。可能是会话已被删除，或者您没有访问权限。
- **图标：** 🔒

### deleted
- **标题：** 会话已删除
- **描述：** 这个会话已经被删除了喵。不过没关系，您可以创建一个新的会话继续对话。
- **图标：** 🗑️

---

## ♿ 可访问性特性

### ARIA 属性
```tsx
<div
  role="alert"           // 标记为警告区域
  aria-live="polite"     // 内容变化时通知读屏软件
>
```

### 语义化 HTML
- 使用 `<h1>` 标记错误标题
- 使用 `<p>` 标记错误描述
- 使用 `<ul>` 和 `<li>` 标记辅助提示列表

### 键盘导航
- 所有按钮都可以通过 Tab 键访问
- 按钮有清晰的焦点指示器

---

## 📱 响应式设计

组件在不同屏幕尺寸下自动适配：

- **移动端：** 按钮垂直排列（`flex-col`）
- **桌面端：** 按钮水平排列（`sm:flex-row`）

```tsx
<div className="flex flex-col gap-3 sm:flex-row sm:justify-center">
  {/* 按钮 */}
</div>
```

---

## 🧪 测试建议

### 单元测试

```typescript
import { render, screen, fireEvent } from '@testing-library/react';
import { SessionNotFound } from './SessionNotFound';

describe('SessionNotFound', () => {
  it('应该显示默认错误消息', () => {
    render(<SessionNotFound />);
    expect(screen.getByText('会话不存在')).toBeInTheDocument();
  });

  it('应该调用 onGoHome 回调', () => {
    const onGoHome = jest.fn();
    render(<SessionNotFound onGoHome={onGoHome} />);

    fireEvent.click(screen.getByText('返回首页'));
    expect(onGoHome).toHaveBeenCalled();
  });

  it('应该显示自定义消息', () => {
    const customMessage = '自定义错误消息';
    render(<SessionNotFound message={customMessage} />);

    expect(screen.getByText(customMessage)).toBeInTheDocument();
  });

  it('应该有正确的 ARIA 属性', () => {
    const { container } = render(<SessionNotFound />);
    const alert = container.querySelector('[role="alert"]');

    expect(alert).toHaveAttribute('aria-live', 'polite');
  });
});
```

### 可访问性测试

```typescript
import { axe } from 'jest-axe';

it('应该没有可访问性问题', async () => {
  const { container } = render(<SessionNotFound />);
  const results = await axe(container);

  expect(results).toHaveNoViolations();
});
```

---

## 🎨 样式定制

如果需要自定义样式，可以通过修改组件内的 Tailwind 类名：

```tsx
// 修改背景色
<div className="flex h-full w-full items-center justify-center bg-custom-color p-8">

// 修改图标大小
<div className="flex h-40 w-40 items-center justify-center rounded-full bg-gray-100">

// 修改文字颜色
<h1 className="mb-3 text-2xl font-bold text-custom-color">
```

---

## 📋 最佳实践

1. **总是提供返回首页按钮**
   - 让用户有明确的退出路径

2. **根据错误类型选择合适的 errorType**
   - 提供准确的错误信息

3. **在加载状态时显示加载指示器**
   - 避免过早显示错误页面

4. **记录错误日志**
   - 帮助排查问题

```tsx
if (!session) {
  console.error('Session not found:', sessionId);
  // 或使用错误追踪服务
  // Sentry.captureException(new Error('Session not found'));

  return <SessionNotFound />;
}
```

---

## 🔗 相关组件

- `Button` - 按钮组件
- `LoadingSpinner` - 加载指示器
- `ErrorBoundary` - 错误边界

---

## 📞 问题反馈

如有问题或建议，请联系：
- **技术问题：** 花花（主架构师）
- **设计问题：** 小乔（UI/UX 设计师）

---

**最后更新：** 2026-02-18
**维护者：** 三花猫·花花 🐱
