# Claude-Orange UI Redesign Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 将整个 UI 从当前靛蓝色(indigo)配色切换为 Claude 风格暖橙色配色，header 改为深黑色，增加整体间距使布局更稀疏舒适。

**Architecture:** CSS 变量驱动 — 修改 `index.css` `:root` 中的颜色变量即可覆盖全局配色，再配合 `App.css` 和 `App.tsx` 调整间距布局。所有组件 CSS 文件已使用 CSS 变量，无需逐个修改。

**Tech Stack:** React 18, Ant Design 5, CSS Custom Properties

**Risks:**
- Ant Design 内部部分组件可能不响应 CSS 变量覆盖 → 缓解：通过 `!important` 全局覆盖已验证有效
- 暖色调可能导致文字对比度不足 → 缓解：使用暖黑色 `#1C1917` 而非纯黑

---

### Task 1: Update Root Color Variables and Global Styles

**Depends on:** None
**Files:**
- Modify: `frontend/src/index.css:1-112`

- [ ] **Step 1: 替换 `:root` 颜色变量为 Claude 暖橙色调**

文件: `frontend/src/index.css:1-43`（替换整个 `:root` 块）

```css
:root {
  /* Primary palette - warm neutral (Claude style) */
  --color-bg: #F5F3F0;
  --color-surface: #FFFFFF;
  --color-border: #E2DED8;
  --color-border-light: #EDEAE5;

  /* Text - warm tones */
  --color-text-primary: #1C1917;
  --color-text-secondary: #6B6560;
  --color-text-tertiary: #A39E99;

  /* Accent - Claude warm orange */
  --color-accent: #D97706;
  --color-accent-hover: #B45309;
  --color-accent-light: #FFF7ED;
  --color-accent-bg: #FFFBEB;

  /* Semantic */
  --color-success: #16a34a;
  --color-success-bg: #f0fdf4;
  --color-warning: #d97706;
  --color-warning-bg: #fffbeb;
  --color-error: #dc2626;
  --color-error-bg: #fef2f2;
  --color-info: #0284c7;
  --color-info-bg: #f0f9ff;

  /* Layout - Claude dark header */
  --color-header-bg: #1A1A1A;
  --color-header-text: #E8E6E3;
  --color-sider-bg: #FAFAF8;
  --color-sider-active: var(--color-accent-light);

  /* Radius - soft rounded */
  --radius-sm: 6px;
  --radius-md: 8px;
  --radius-lg: 12px;

  /* Shadows - warm and subtle */
  --shadow-sm: 0 1px 2px rgba(28, 25, 23, 0.05);
  --shadow-md: 0 2px 6px rgba(28, 25, 23, 0.07);
}
```

- [ ] **Step 2: 更新全局 body 和 Ant Design 覆盖样式，增加圆角和间距**

文件: `frontend/src/index.css:44-112`（替换全局样式块）

保持 body 和 code 样式不变，增加 Ant Design 覆盖中的圆角和柔和度。

---

### Task 2: Update Layout Spacing and Header Style

**Depends on:** Task 1
**Files:**
- Modify: `frontend/src/App.css:1-59`
- Modify: `frontend/src/App.tsx:78-118`

- [ ] **Step 1: 更新 App.css — 增加 sidebar 和 header 的间距和舒适度**

- [ ] **Step 2: 更新 App.tsx — 增加 Content padding 和 Sider width 使布局更稀疏**

---

### Task 3: Verify and Fine-tune Component CSS

**Depends on:** Task 1, Task 2
**Files:**
- Review: all 5 component CSS files (already use CSS variables, should auto-update)

- [ ] **Step 1: 检查所有组件 CSS 文件是否有硬编码的紫色或靛蓝色值**
- [ ] **Step 2: 验证前端页面渲染效果**
