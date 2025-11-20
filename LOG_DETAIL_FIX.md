# 日志详情功能修复

## 问题描述
点击请求日志的"详情"按钮时，弹窗无法正常显示或出现错误。

## 问题原因
1. JSON解析错误：直接使用`JSON.parse()`可能因为数据格式问题导致异常
2. 缺少错误处理：没有try-catch保护
3. 缺少空值判断：当request_body或response_body为空时可能出错
4. 弹窗交互性差：无法点击遮罩关闭

## 解决方案

### 修改文件
`frontend/src/components/RequestLogs.tsx`

### 主要改进

#### 1. 安全的JSON解析
```typescript
// 之前（可能抛出异常）
value={JSON.stringify(JSON.parse(record.request_body), null, 2)}

// 修复后（安全解析）
let requestBodyDisplay = '';
if (record.request_body) {
  try {
    const parsed = JSON.parse(record.request_body);
    requestBodyDisplay = JSON.stringify(parsed, null, 2);
  } catch (e) {
    requestBodyDisplay = record.request_body; // 解析失败直接显示原文
  }
}
```

#### 2. 添加更多信息显示
- 请求摘要（request_summary）
- 响应预览（response_preview）
- 错误信息（error_message）
- 完整请求体
- 完整响应体

#### 3. 改进交互体验
```typescript
Modal.info({
  title: '请求详情',
  width: 900,
  maskClosable: true, // 允许点击遮罩关闭
  content: (...)
});
```

#### 4. 空数据提示
```typescript
{!requestBodyDisplay && !responseBodyDisplay && !record.error_message && (
  <div style={{ marginTop: 16, padding: 16, background: '#f5f5f5', borderRadius: 4 }}>
    <Text type="secondary">暂无详细信息</Text>
  </div>
)}
```

## 测试步骤

1. **访问日志页面**
   ```
   http://localhost:8083/ui/configs/{config-id}?tab=logs
   ```

2. **点击"详情"按钮**
   - 应该弹出详情弹窗
   - 显示基本信息（ID、时间、模型、状态等）

3. **查看详细内容**
   - 如果有请求摘要，显示摘要
   - 如果有响应预览，显示预览
   - 如果有错误信息，以红色显示
   - 如果有完整请求/响应体，格式化显示JSON

4. **关闭弹窗**
   - 点击"确定"按钮可关闭
   - 点击弹窗外的遮罩可关闭
   - 按ESC键可关闭

## 详情弹窗显示内容

### 基本信息（始终显示）
- ID
- 时间
- 模型
- 状态（成功/失败标签）
- 耗时（毫秒）
- 输入Token
- 输出Token

### 可选信息（如果有数据）
1. **错误信息**（失败时）
   - 红色文字显示
   - 支持多行文本

2. **请求摘要**
   - 简要的请求内容描述

3. **响应预览**
   - 简要的响应内容预览

4. **完整请求体**
   - JSON格式化显示
   - 只读文本框
   - 支持滚动查看

5. **完整响应体**
   - JSON格式化显示
   - 只读文本框
   - 支持滚动查看

6. **无详细信息提示**
   - 当没有任何额外信息时显示灰色提示框

## 构建和部署

```bash
# 构建前端
cd frontend
npm run build

# 刷新浏览器页面
# http://localhost:8083/ui/configs/{config-id}?tab=logs
```

## 验证清单

- ✅ 点击"详情"按钮弹出弹窗
- ✅ 弹窗显示基本信息
- ✅ JSON安全解析不报错
- ✅ 成功日志显示完整请求/响应
- ✅ 失败日志显示错误信息
- ✅ 点击遮罩可关闭弹窗
- ✅ 没有数据时显示友好提示

## 修复时间
2025-11-20 01:22 UTC+08:00

## 相关文件
- `frontend/src/components/RequestLogs.tsx` - 日志列表组件（已修复）
