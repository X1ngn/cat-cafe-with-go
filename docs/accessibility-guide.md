# 可访问性组件使用指南

## 概述

本项目已实现符合 WCAG 2.1 AA 标准的可访问性组件库，解决了薇薇审查报告中的所有问题。

---

## 📦 已修复的问题

### ✅ 严重问题
- **焦点陷阱缺失** - Modal 和 Drawer 组件现在支持完整的焦点管理
- **表单错误处理** - Form 组件自动聚焦到第一个错误字段

### ✅ 中等问题
- **触控热区过小** - IconButton 确保最小 44x44px 热区
- **输入框对比度** - Input 组件使用 `text-gray-500` 确保 4.5:1 对比度
- **图片语义化** - 提供 AccessibleImage、ProductImage、AvatarImage 组件

### ✅ 建议项
- **读屏播报** - 实现 useAnnouncer Hook 和全局 AnnouncerProvider

---

## 🚀 快速开始

### 1. 在应用根组件添加 AnnouncerProvider

```tsx
import { AnnouncerProvider } from './a11y';

function App() {
  return (
    <AnnouncerProvider>
      <YourApp />
    </AnnouncerProvider>
  );
}
```

### 2. 使用 Modal 组件

```tsx
import { Modal } from './a11y';

function ProductModal({ isOpen, onClose, product }) {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="商品详情"
    >
      <div>
        <h3>{product.name}</h3>
        <p>{product.description}</p>
        <button onClick={onClose}>关闭</button>
      </div>
    </Modal>
  );
}
```

**特性：**
- ✅ 自动焦点陷阱（Tab 键循环）
- ✅ Esc 键关闭
- ✅ 关闭后焦点返回触发元素
- ✅ 关闭按钮触控热区 ≥ 44x44px

---

### 3. 使用 Drawer 组件

```tsx
import { Drawer } from './a11y';

function CartDrawer({ isOpen, onClose, items }) {
  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title="购物车"
      placement="right"
    >
      <ul>
        {items.map(item => (
          <li key={item.id}>{item.name}</li>
        ))}
      </ul>
    </Drawer>
  );
}
```

---

### 4. 使用 Form 和 Input 组件

```tsx
import { Form, Input } from './a11y';
import { useState } from 'react';

function CheckoutForm() {
  const [errors, setErrors] = useState({});

  const handleSubmit = (e) => {
    const formData = new FormData(e.currentTarget);
    const phone = formData.get('phone');

    if (!phone) {
      setErrors({ phone: '请输入手机号' });
      return;
    }

    // 提交订单...
  };

  return (
    <Form onSubmit={handleSubmit} errors={errors}>
      <Input
        name="phone"
        label="手机号"
        required
        error={errors.phone}
        placeholder="请输入手机号"
      />

      <Input
        name="address"
        label="配送地址"
        required
        error={errors.address}
        helperText="请填写详细地址"
      />

      <button type="submit">提交订单</button>
    </Form>
  );
}
```

**特性：**
- ✅ 错误时自动聚焦到第一个错误字段
- ✅ 错误信息通过 `aria-describedby` 关联
- ✅ 必填项标记 `*` 和读屏文本 `(必填)`
- ✅ Placeholder 对比度符合 WCAG AA

---

### 5. 使用 IconButton 组件

```tsx
import { IconButton } from './a11y';

function CartItem({ item, onRemove }) {
  return (
    <div>
      <span>{item.name}</span>
      <IconButton
        aria-label={`删除 ${item.name}`}
        onClick={() => onRemove(item.id)}
        icon={<TrashIcon />}
      />
    </div>
  );
}
```

**特性：**
- ✅ 必须提供 `aria-label`
- ✅ 触控热区 ≥ 44x44px
- ✅ 支持键盘聚焦和操作

---

### 6. 使用图片组件

```tsx
import { ProductImage, AvatarImage, AccessibleImage } from './a11y';

// 商品图片
<ProductImage
  productName="猫爪拿铁"
  src="/images/latte.jpg"
  className="w-full h-48 object-cover"
/>

// 用户头像
<AvatarImage
  userName="张三"
  src="/avatars/user1.jpg"
  className="w-10 h-10 rounded-full"
/>

// 装饰性图片
<AccessibleImage
  src="/decorations/pattern.svg"
  alt=""
  decorative
/>
```

**特性：**
- ✅ 强制要求有意义的 alt 文本
- ✅ 装饰性图片使用 `aria-hidden="true"`
- ✅ 开发时警告缺失 alt

---

### 7. 使用读屏播报

```tsx
import { useGlobalAnnouncer } from './a11y';

function CartDrawer({ items, onRemoveItem }) {
  const { announce } = useGlobalAnnouncer();

  const handleRemove = (item) => {
    onRemoveItem(item.id);
    announce(`${item.name} 已从购物车移除`, 'polite');
  };

  return (
    <div>
      {items.map(item => (
        <div key={item.id}>
          <span>{item.name}</span>
          <button onClick={() => handleRemove(item)}>删除</button>
        </div>
      ))}
    </div>
  );
}
```

**特性：**
- ✅ `polite` - 礼貌播报（不打断当前朗读）
- ✅ `assertive` - 强制播报（立即打断）

---

## 🎨 Tailwind CSS 配置

确保在 `tailwind.config.js` 中添加 `sr-only` 类：

```js
module.exports = {
  theme: {
    extend: {},
  },
  plugins: [
    function({ addUtilities }) {
      addUtilities({
        '.sr-only': {
          position: 'absolute',
          width: '1px',
          height: '1px',
          padding: '0',
          margin: '-1px',
          overflow: 'hidden',
          clip: 'rect(0, 0, 0, 0)',
          whiteSpace: 'nowrap',
          borderWidth: '0',
        },
      });
    },
  ],
};
```

---

## 📋 可访问性检查清单

在开发时，请确保：

- [ ] 所有模态框/抽屉使用 Modal/Drawer 组件
- [ ] 所有表单使用 Form + Input 组件
- [ ] 所有图标按钮提供 `aria-label`
- [ ] 所有图片提供有意义的 `alt` 文本
- [ ] 重要状态变化使用 `announce()` 播报
- [ ] 触控目标 ≥ 44x44px
- [ ] 文本对比度 ≥ 4.5:1

---

## 🧪 测试建议

### 键盘导航测试
1. 使用 `Tab` 键遍历所有可交互元素
2. 确保焦点可见（focus ring）
3. 在模态框中按 `Tab`，焦点不应逃逸
4. 按 `Esc` 键应关闭模态框

### 读屏软件测试
- macOS: VoiceOver (`Cmd + F5`)
- Windows: NVDA (免费)
- 确保所有信息都能被正确朗读

### 触控测试
- 在移动设备上测试所有按钮
- 确保不会误触

---

## 📚 参考资源

- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/)

---

喵~ 现在我们的系统对所有用户都友好了！🐱✨
