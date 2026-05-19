# Config Detail Page UI Redesign

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 重新设计配置详情页的布局和信息展示：引入语法高亮代码块、优化 Banner/Title 设计、改善整体排版。

**Architecture:** 保留 ConfigDetailV2.tsx 所有现有 state/logic（API 调用、modal 交互），只重构 JSX 渲染输出。创建内联 CodeBlock 组件封装 react-syntax-highlighter，替换所有 Input.TextArea 代码展示。

**Tech Stack:** React 18, Ant Design 5, react-syntax-highlighter 16.1.0 (已安装), TypeScript

**Risks:**
- Task 3 大幅修改渲染代码，可能遗漏某个交互 → 缓解：逐段替换，保留所有按钮回调

---

### Task 1: Add Syntax-Highlighted CodeBlock Component

**Depends on:** None
**Files:**
- Modify: `frontend/src/components/ConfigDetailV2.tsx:1-34`（import 区块 + 新增组件）

- [ ] **Step 1: 添加 import 和 CodeBlock 组件 — 封装 react-syntax-highlighter 为可复用代码块**

文件: `frontend/src/components/ConfigDetailV2.tsx:1-34`（替换 import 区块并添加 CodeBlock 组件）

```typescript
import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { usePageTitle } from '../utils/pageTitle';
import { getApiOrigin } from '../services/apiBase';
import {
  Card, Descriptions, Button, Space, message, Modal, Tag, Spin, Tabs,
  Typography, Input, Tooltip,
} from 'antd';
import {
  ArrowLeftOutlined, EditOutlined, DeleteOutlined, ReloadOutlined,
  CopyOutlined, SyncOutlined,
} from '@ant-design/icons';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import axios from 'axios';
import RequestLogs from './RequestLogs';
import ConfigTestInline from './ConfigTestInline';

const { Paragraph, Text } = Typography;

const CodeBlock: React.FC<{
  code: string;
  language?: string;
  title?: string;
}> = ({ code, language = 'bash', title }) => (
  <div style={{ position: 'relative', borderRadius: 8, overflow: 'hidden', marginBottom: 4 }}>
    {title && (
      <div style={{
        background: '#2d2d2d', color: '#ccc', padding: '6px 12px',
        fontSize: 12, fontFamily: 'monospace', borderBottom: '1px solid #444',
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
      }}>
        <span>{title}</span>
        <Button
          type="text" size="small" icon={<CopyOutlined />}
          style={{ color: '#999', fontSize: 11 }}
          onClick={() => { navigator.clipboard.writeText(code); message.success('已复制'); }}
        >
          复制
        </Button>
      </div>
    )}
    {!title && (
      <Button
        type="text" size="small" icon={<CopyOutlined />}
        style={{ position: 'absolute', top: 6, right: 6, color: '#666', zIndex: 1 }}
        onClick={() => { navigator.clipboard.writeText(code); message.success('已复制'); }}
      />
    )}
    <SyntaxHighlighter
      language={language} style={oneDark}
      customStyle={{
        margin: 0, borderRadius: title ? 0 : 8,
        fontSize: 13, lineHeight: 1.6,
        padding: '12px 16px',
      }}
    >
      {code}
    </SyntaxHighlighter>
  </div>
);
```

- [ ] **Step 2: 验证编译**
Run: `cd frontend && npx tsc --noEmit 2>&1 | head -5`
Expected: 无 import 相关错误

---

### Task 2: Redesign Page Header

**Depends on:** Task 1
**Files:**
- Modify: `frontend/src/components/ConfigDetailV2.tsx:367-430`（替换 header/banner 渲染区块）

- [ ] **Step 1: 重设计 Banner — 用紧凑信息头替代全宽渐变 Banner**

文件: `frontend/src/components/ConfigDetailV2.tsx:367-430`（替换从 `<div>` 开始到 banner `</div>` 结束的区域）

将原来的大渐变 Banner 替换为：
- 左侧：配置名 + 状态 + 关键元数据（创建时间、Base URL）
- 右侧：操作按钮组
- 底部：客户端状态条（紧凑版，非全宽渐变）
- 不再使用 `📋` emoji，用 Ant Design 图标替代

```tsx
      {/* 页面头部 — 紧凑布局 */}
      <div style={{
        marginBottom: 20, display: 'flex', justifyContent: 'space-between',
        alignItems: 'flex-start', gap: 16, flexWrap: 'wrap',
      }}>
        <div style={{ flex: '1 1 auto', minWidth: 300 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <Button
              type="text" icon={<ArrowLeftOutlined />}
              onClick={() => navigate('/ui')}
              style={{ marginLeft: -8, color: 'var(--color-text-secondary)' }}
            />
            <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: 'var(--color-text-primary)' }}>
              {config.name}
            </h2>
            <Tag color={config.enabled ? 'success' : 'default'} style={{ fontSize: 12 }}>
              {config.enabled ? '启用' : '禁用'}
            </Tag>
          </div>
          {config.description && (
            <Text type="secondary" style={{ fontSize: 13, marginLeft: 42 }}>
              {config.description}
            </Text>
          )}
          <div style={{ display: 'flex', gap: 16, marginTop: 8, marginLeft: 42, flexWrap: 'wrap' }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              ID: {config.id}
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              创建于 {new Date(config.created_at).toLocaleDateString('zh-CN')}
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              Base URL: {config.openai_base_url}
            </Text>
          </div>
        </div>
        <Space>
          <Button type="primary" icon={<EditOutlined />}
            onClick={() => navigate(`/ui/configs/${id}/edit`)}>
            编辑
          </Button>
          <Button icon={<ReloadOutlined />} onClick={fetchConfigDetail}>刷新</Button>
          <Button danger icon={<DeleteOutlined />} onClick={handleDelete}>删除</Button>
        </Space>
      </div>

      {/* 状态提示条 — 紧凑行内样式 */}
      {clientStats && (
        <div style={{ marginBottom: 12 }}>
          {clientStats.active_clients > 0 ? (
            <div style={{
              padding: '8px 16px', borderRadius: 8,
              background: 'var(--color-success-bg)',
              border: '1px solid #bbf7d0',
              display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
            }}>
              <Tag color="success" style={{ margin: 0 }}>活跃</Tag>
              <span>
                {clientStats.has_concurrent
                  ? `至少 ${clientStats.estimated_clients} 个客户端在线`
                  : `${clientStats.active_clients} 个客户端在线`}
              </span>
              <span style={{ color: 'var(--color-text-tertiary)', fontSize: 12, marginLeft: 'auto' }}>
                最后请求: {clientStats.last_request_at ? new Date(clientStats.last_request_at).toLocaleTimeString('zh-CN') : '-'}
              </span>
            </div>
          ) : clientStats.total_clients > 0 ? (
            <div style={{
              padding: '8px 16px', borderRadius: 8,
              background: 'var(--color-warning-bg)',
              border: '1px solid #fed7aa',
              display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
            }}>
              <Tag color="warning" style={{ margin: 0 }}>空闲</Tag>
              <span>24h 内 {clientStats.total_clients} 个客户端使用过</span>
              <span style={{ color: 'var(--color-text-tertiary)', fontSize: 12, marginLeft: 'auto' }}>
                最后请求: {clientStats.last_request_at ? new Date(clientStats.last_request_at).toLocaleTimeString('zh-CN') : '-'}
              </span>
            </div>
          ) : (
            <div style={{
              padding: '8px 16px', borderRadius: 8,
              background: 'var(--color-border-light)',
              display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
            }}>
              <Tag style={{ margin: 0 }}>未使用</Tag>
              <span style={{ color: 'var(--color-text-tertiary)' }}>最近 24h 无使用记录</span>
            </div>
          )}
        </div>
      )}

      {/* 过期警告 */}
      {config.expires_at && new Date(config.expires_at) < new Date() && (
        <div style={{
          marginBottom: 12, padding: '8px 16px', borderRadius: 8,
          background: 'var(--color-error-bg)', border: '1px solid #fecaca',
          display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
        }}>
          <Tag color="error" style={{ margin: 0 }}>已过期</Tag>
          <span>密钥已于 {new Date(config.expires_at).toLocaleString('zh-CN')} 过期，API 调用将被拒绝</span>
        </div>
      )}
```

- [ ] **Step 2: 验证编译**
Run: `cd frontend && npx tsc --noEmit 2>&1 | head -5`
Expected: 无错误

---

### Task 3: Reorganize Content with Syntax Highlighting

**Depends on:** Task 2
**Files:**
- Modify: `frontend/src/components/ConfigDetailV2.tsx:551-843`（替换 Overview tab children 内容）

- [ ] **Step 1: 重布局 Overview 内容 — 双栏布局 + 语法高亮代码块**

文件: `frontend/src/components/ConfigDetailV2.tsx:551-843`（替换 Overview tab children 区域）

主要改动：
- 基本信息和 OpenAI 配置合为一行双栏
- Anthropic Token 卡片更紧凑
- CLI 配置所有 Input.TextArea 替换为 CodeBlock 组件（bash 语法高亮）
- 去掉重复的复制按钮（CodeBlock 自带）

```tsx
              <>
                {/* 双栏：基本信息 + OpenAI 配置 */}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 16 }}>
                  <Card title="基本信息" size="small">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="配置 ID">
                        <Text copyable style={{ fontFamily: 'monospace', fontSize: 12 }}>{config.id}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="状态">
                        <Tag color={config.enabled ? 'success' : 'default'}>
                          {config.enabled ? '启用' : '禁用'}
                        </Tag>
                      </Descriptions.Item>
                      <Descriptions.Item label="创建时间">
                        {new Date(config.created_at).toLocaleString('zh-CN')}
                      </Descriptions.Item>
                      <Descriptions.Item label="更新时间">
                        {new Date(config.updated_at).toLocaleString('zh-CN')}
                      </Descriptions.Item>
                      <Descriptions.Item label="过期时间">
                        {config.expires_at ? (
                          <span>
                            {new Date(config.expires_at).toLocaleString('zh-CN')}
                            {new Date(config.expires_at) < new Date()
                              ? <Tag color="error" style={{ marginLeft: 8 }}>已过期</Tag>
                              : <Tag color="success" style={{ marginLeft: 8 }}>有效</Tag>
                            }
                          </span>
                        ) : (
                          <span style={{ color: 'var(--color-text-tertiary)' }}>永久有效</span>
                        )}
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>

                  <Card title="OpenAI 配置" size="small">
                    <Descriptions column={1} size="small">
                      <Descriptions.Item label="API Key">
                        <Text style={{ fontFamily: 'monospace', fontSize: 12 }}>
                          {config.openai_api_key_masked}
                        </Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="Base URL">
                        <Text copyable style={{ fontFamily: 'monospace', fontSize: 12 }}>
                          {config.openai_base_url}
                        </Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="大模型">
                        <Text strong>{config.big_model}</Text>
                        {config.big_model_reasoning_effort && (
                          <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>
                            {config.big_model_reasoning_effort.toUpperCase()}
                          </Tag>
                        )}
                      </Descriptions.Item>
                      <Descriptions.Item label="中模型">
                        <Text strong>{config.middle_model}</Text>
                        {config.middle_model_reasoning_effort && (
                          <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>
                            {config.middle_model_reasoning_effort.toUpperCase()}
                          </Tag>
                        )}
                      </Descriptions.Item>
                      <Descriptions.Item label="小模型">
                        <Text strong>{config.small_model}</Text>
                        {config.small_model_reasoning_effort && (
                          <Tag color="blue" style={{ marginLeft: 6, fontSize: 11 }}>
                            {config.small_model_reasoning_effort.toUpperCase()}
                          </Tag>
                        )}
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </div>

                {/* 参数配置 — 紧凑横排 */}
                <Card title="请求参数" size="small" style={{ marginBottom: 16 }}>
                  <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap' }}>
                    {[
                      { label: '最大 Token', value: config.max_tokens_limit },
                      { label: '超时', value: `${config.request_timeout}s` },
                      { label: '重试次数', value: config.retry_count || 3 },
                      { label: '退避基数', value: `${config.retry_backoff_base || 1}s` },
                      { label: '退避上限', value: `${config.retry_backoff_max || 60}s` },
                      { label: '思考级别', value: config.reasoning_effort ? config.reasoning_effort.toUpperCase() : '默认' },
                    ].map(item => (
                      <div key={item.label} style={{ textAlign: 'center', minWidth: 80 }}>
                        <div style={{ fontSize: 11, color: 'var(--color-text-tertiary)', marginBottom: 2 }}>
                          {item.label}
                        </div>
                        <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-text-primary)' }}>
                          {item.value}
                        </div>
                      </div>
                    ))}
                  </div>
                </Card>

                {/* Anthropic API Token */}
                <Card
                  title="Anthropic API Token"
                  size="small"
                  extra={
                    <Space size="small">
                      <Tooltip title="自动生成 UUID Token">
                        <Button type="link" icon={<SyncOutlined spin={renewingKey} />}
                          onClick={handleRenewKey} loading={renewingKey} size="small">
                          更新
                        </Button>
                      </Tooltip>
                      <Tooltip title="自定义 Token">
                        <Button type="link" icon={<EditOutlined />}
                          onClick={handleCustomToken} loading={renewingKey} size="small">
                          自定义
                        </Button>
                      </Tooltip>
                    </Space>
                  }
                  style={{ marginBottom: 16 }}
                >
                  <CodeBlock code={anthropicApiKey} language="text" />
                  <Text type="secondary" style={{ fontSize: 12, marginTop: 8, display: 'block' }}>
                    使用此 Token 作为 ANTHROPIC_API_KEY
                  </Text>
                </Card>

                {/* CLI 配置 — 语法高亮 */}
                <Card title="Claude Code CLI 配置" size="small">
                  <Space direction="vertical" style={{ width: '100%' }} size="middle">
                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        单次执行（推荐）
                      </Text>
                      <CodeBlock code={oneTimeCommand} language="bash" title="bash" />
                    </div>

                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        永久配置（添加到 ~/.zshrc）
                      </Text>
                      <CodeBlock code={persistentConfig} language="bash" title="~/.zshrc" />
                    </div>

                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        一键配置脚本
                      </Text>
                      <CodeBlock code={oneClickSetupScript} language="bash" title="one-click setup" />
                      <Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 4 }}>
                        使用 bash 的话，将 ~/.zshrc 改为 ~/.bashrc
                      </Text>
                    </div>

                    <div>
                      <Text strong style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>
                        带提示词执行
                      </Text>
                      <Input
                        placeholder="输入提示词，例如：Hello, Claude!"
                        value={promptText}
                        onChange={(e) => setPromptText(e.target.value)}
                        style={{ marginBottom: 8 }}
                        suffix={promptText && (
                          <Button type="text" size="small"
                            onClick={() => setPromptText('')}
                            style={{ padding: 0, height: 'auto' }}>
                            清空
                          </Button>
                        )}
                      />
                      <CodeBlock code={promptCommand} language="bash" title="bash -p" />
                    </div>

                    <div style={{
                      padding: 12, borderRadius: 8,
                      background: 'var(--color-warning-bg)', border: '1px solid #fed7aa',
                    }}>
                      <Text style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                        <Text strong>--dangerously-skip-permissions</Text> 跳过权限确认，适合自动化
                      </Text>
                      <Text style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                        <Text strong>CLAUDE_CODE_MAX_OUTPUT_TOKENS={config.max_tokens_limit}</Text> 与 OpenAI 配置一致
                      </Text>
                      <Text style={{ fontSize: 12, display: 'block' }}>
                        <Text strong>-p "提示词"</Text> 直接传递提示词
                      </Text>
                    </div>
                  </Space>
                </Card>
              </>
```

- [ ] **Step 2: 验证前端编译并预览**
Run: `cd frontend && npx tsc --noEmit 2>&1 | head -10`
Expected: Exit code 0, no errors

- [ ] **Step 3: 提交**
Run: `git add frontend/src/components/ConfigDetailV2.tsx && git commit -m "feat(ui): redesign config detail page with syntax highlighting and improved layout"`
