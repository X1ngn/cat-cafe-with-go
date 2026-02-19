# 可访问性测试指南

本指南帮助开发者和测试人员验证组件是否符合 WCAG 2.1 AA 标准。

---

## 📋 测试清单

### 1. 焦点陷阱测试 (Modal & Drawer)

#### 手动测试步骤

**测试工具：** 键盘

**步骤：**
1. 打开模态框或抽屉
2. 按 `Tab` 键 20 次，观察焦点是否始终在组件内循环
3. 按 `Shift+Tab` 键 20 次，观察焦点是否反向循环
4. 按 `Esc` 键，确认组件关闭
5. 确认焦点返回到触发按钮

**验收标准：**
- ✅ 焦点不逃逸到背景内容
- ✅ `Esc` 键可关闭
- ✅ 关闭后焦点正确返回

**读屏软件测试：**
- 使用 NVDA (Windows) 或 VoiceOver (macOS)
- 确认能听到 "对话框" 或 "抽屉" 的角色播报
- 确认能听到标题内容

---

### 2. 表单错误处理测试 (Form & Input)

#### 手动测试步骤

**测试工具：** 键盘 + 读屏软件

**步骤：**
1. 不填写必填项，提交表单
2. 观察焦点是否自动跳转到第一个错误字段
3. 使用读屏软件，确认能听到错误信息

**验收标准：**
- ✅ 焦点自动跳转到第一个错误字段
- ✅ 错误字段边框变红
- ✅ 读屏软件播报："字段名，必填，错误：错误信息"

**代码检查：**
```tsx
// 确认以下属性存在
<input
  aria-invalid="true"
  aria-describedby="error-id"
/>
<p id="error-id" role="alert">错误信息</p>
```

---

### 3. 触控热区测试 (IconButton)

#### 手动测试步骤

**测试工具：** Chrome DevTools + 移动设备

**步骤：**
1. 打开 Chrome DevTools，切换到移动设备模拟
2. 选择 iPhone SE (375px 宽)
3. 尝试点击所有图标按钮
4. 使用 DevTools 的 "Show rulers" 功能测量热区

**验收标准：**
- ✅ 所有图标按钮热区 ≥ 44x44px
- ✅ 在小屏设备上能轻松点击
- ✅ 不会误触相邻按钮

**代码检查：**
```tsx
// 确认按钮包含以下类名
className="min-w-[44px] min-h-[44px]"
```

---

### 4. 颜色对比度测试 (Input)

#### 手动测试步骤

**测试工具：** WebAIM Contrast Checker

**步骤：**
1. 访问 https://webaim.org/resources/contrastchecker/
2. 输入前景色和背景色的十六进制值
3. 确认对比度 ≥ 4.5:1

**验收标准：**
- ✅ 占位符文本对比度 ≥ 4.5:1
- ✅ 标签文本对比度 ≥ 4.5:1
- ✅ 错误信息对比度 ≥ 4.5:1

**推荐颜色值：**
- 占位符：`text-gray-500` (#6B7280)
- 标签：`text-gray-700` (#374151)
- 错误：`text-red-600` (#DC2626)

---

### 5. 图片替代文本测试

#### 手动测试步骤

**测试工具：** 读屏软件 + axe DevTools

**步骤：**
1. 使用 NVDA/VoiceOver 浏览页面
2. 确认商品图片播报有意义的描述
3. 使用 axe DevTools 扫描页面

**验收标准：**
- ✅ 商品图片有描述性 `alt` 文本
- ✅ 装饰性图片使用 `alt=""` 或 `role="presentation"`
- ✅ axe DevTools 无图片相关警告

**代码检查：**
```tsx
// 正确示例
<img src="coffee.jpg" alt="猫爪咖啡" />

// 装饰性图片
<img src="decoration.jpg" alt="" role="presentation" />
```

---

### 6. 状态播报测试 (useAnnouncer)

#### 手动测试步骤

**测试工具：** 读屏软件

**步骤：**
1. 使用 NVDA/VoiceOver
2. 执行操作（如删除购物车商品）
3. 确认能听到状态变化播报

**验收标准：**
- ✅ 操作后 500ms 内听到播报
- ✅ 播报内容清晰明确
- ✅ 不打断当前正在播报的内容

**代码检查：**
```tsx
const { announce } = useAnnouncer();

// 使用示例
announce('商品已删除', 'polite');
announce('表单提交失败', 'assertive');
```

---

## 🛠 推荐测试工具

### 浏览器扩展
- **axe DevTools** - 自动化可访问性扫描
- **WAVE** - 可视化可访问性评估
- **Lighthouse** - Chrome 内置审计工具

### 读屏软件
- **NVDA** (Windows) - 免费开源
- **JAWS** (Windows) - 商业软件
- **VoiceOver** (macOS/iOS) - 系统内置

### 对比度检查
- **WebAIM Contrast Checker** - https://webaim.org/resources/contrastchecker/
- **Colour Contrast Analyser** - 桌面应用

### 键盘测试
- 只使用键盘（不使用鼠标）完成所有操作
- 确认焦点可见且逻辑清晰

---

## 📊 自动化测试示例

### 使用 Jest + Testing Library

```typescript
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Modal } from '../ui/Modal';

describe('Modal 可访问性测试', () => {
  it('应该有正确的 ARIA 属性', () => {
    render(
      <Modal isOpen={true} onClose={() => {}} title="测试">
        内容
      </Modal>
    );

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAttribute('aria-labelledby');
  });

  it('按 Esc 键应该关闭', async () => {
    const onClose = jest.fn();
    render(
      <Modal isOpen={true} onClose={onClose} title="测试">
        内容
      </Modal>
    );

    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
```

---

## 🎯 测试优先级

### P0 - 必须通过
- 焦点陷阱功能
- 表单错误处理
- 键盘导航

### P1 - 应该通过
- 触控热区
- 颜色对比度
- 图片替代文本

### P2 - 建议通过
- 状态播报
- 动画减弱选项
- 高对比度模式支持

---

## 📝 测试报告模板

```markdown
## 可访问性测试报告

**测试日期：** YYYY-MM-DD
**测试人员：** 姓名
**测试范围：** 组件名称

### 测试结果

| 测试项 | 状态 | 备注 |
|--------|------|------|
| 焦点陷阱 | ✅ 通过 | - |
| 表单错误处理 | ✅ 通过 | - |
| 触控热区 | ❌ 失败 | 关闭按钮热区仅 40x40px |
| 颜色对比度 | ✅ 通过 | - |

### 问题清单

1. **关闭按钮热区不足**
   - 严重级别：中等
   - 复现步骤：...
   - 建议修复：...
```

---

## 🔗 参考资源

- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [MDN Accessibility](https://developer.mozilla.org/en-US/docs/Web/Accessibility)
- [A11y Project Checklist](https://www.a11yproject.com/checklist/)
- [WebAIM Resources](https://webaim.org/resources/)
