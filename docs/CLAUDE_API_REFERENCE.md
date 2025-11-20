# Claude官方API完整参考文档

本文档整理了Claude官方提供的所有API接口，包括请求方法、路径、参数和响应格式，并标注了当前项目的实现状态。

## 目录

- [1. Messages API](#1-messages-api)
- [2. Models API](#2-models-api)
- [3. Batch API](#3-batch-api)
- [4. Files API](#4-files-api)
- [5. Skills API](#5-skills-api)
- [6. Admin API](#6-admin-api)
- [7. 实验性API](#7-实验性api)

---

## 1. Messages API

### 1.1 创建消息 (Create Message)

**实现状态：** ✅ **已实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/messages`
- **描述：** 发送消息给Claude模型，支持流式和非流式响应

**请求头：**
```
x-api-key: <API_KEY>
anthropic-version: 2023-06-01
content-type: application/json
```

**请求体参数：**
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| model | string | 是 | 模型标识符（如"claude-sonnet-4-5"） |
| max_tokens | integer | 是 | 响应的最大token数 |
| messages | array | 是 | 消息数组，包含role和content |
| temperature | number | 否 | 温度参数（0-1），默认1.0 |
| system | string | 否 | 系统提示词 |
| stream | boolean | 否 | 是否启用流式响应 |
| tools | array | 否 | 工具定义数组 |
| metadata | object | 否 | 元数据信息 |

**消息格式：**
```json
{
  "role": "user"|"assistant",
  "content": [
    {
      "type": "text"|"image"|"document"|"tool_result",
      "text": "文本内容"
    }
  ]
}
```

**支持的内容块类型：**
- `text` - 文本内容
- `image` - 图像（Base64或URL）
- `document` - 文档（PDF等）
- `tool_result` - 工具调用结果
- `tool_use` - 工具使用请求

**响应格式（非流式）：**
```json
{
  "id": "msg_01XFDUDYJgAACzvnptvVbrqX",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "响应内容"
    }
  ],
  "model": "claude-sonnet-4-5",
  "stop_reason": "end_turn"|"max_tokens"|"tool_use",
  "usage": {
    "input_tokens": 100,
    "output_tokens": 200
  }
}
```

**流式响应事件类型：**
- `message_start` - 消息开始
- `content_block_start` - 内容块开始
- `content_block_delta` - 内容块增量
- `content_block_stop` - 内容块结束
- `message_delta` - 消息增量（usage更新）
- `message_stop` - 消息结束
- `ping` - 心跳事件
- `error` - 错误事件

---

### 1.2 计算Token数量 (Count Tokens)

**实现状态：** ✅ **已实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/messages/count_tokens`
- **描述：** 在发送请求前计算消息的token数量

**请求参数：** 与创建消息接口相同

**响应格式：**
```json
{
  "input_tokens": 2095
}
```

**项目实现位置：**
- `handler/messages_handler.go` - CountTokens方法

---

## 2. Models API

### 2.1 列出可用模型 (List Models)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/models`
- **描述：** 获取所有可用模型的列表

**查询参数：**
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| before_id | string | 否 | 游标分页-获取此ID之前的结果 |
| after_id | string | 否 | 游标分页-获取此ID之后的结果 |
| limit | integer | 否 | 每页返回数量（1-1000，默认20） |

**响应格式：**
```json
{
  "data": [
    {
      "id": "claude-sonnet-4-20250514",
      "type": "model",
      "display_name": "Claude Sonnet 4",
      "created_at": "2025-02-19T00:00:00Z"
    }
  ],
  "first_id": "claude-sonnet-4-20250514",
  "last_id": "claude-sonnet-4-20250514",
  "has_more": false
}
```

---

### 2.2 获取模型详情 (Get Model)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/models/{model_id}`
- **描述：** 获取指定模型的详细信息

**路径参数：**
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| model_id | string | 是 | 模型标识符或别名 |

**响应格式：**
```json
{
  "id": "claude-sonnet-4-20250514",
  "type": "model",
  "display_name": "Claude Sonnet 4",
  "created_at": "2025-02-19T00:00:00Z"
}
```

---

## 3. Batch API

### 3.1 创建消息批处理 (Create Message Batch)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/messages/batches`
- **描述：** 提交多个消息请求进行异步批处理

**请求体参数：**
```json
{
  "requests": [
    {
      "custom_id": "my-first-request",
      "params": {
        "model": "claude-3-7-sonnet-20250219",
        "max_tokens": 1024,
        "messages": [
          {"role": "user", "content": "Hello, world"}
        ]
      }
    }
  ]
}
```

**响应格式：**
```json
{
  "id": "msgbatch_013Zva2CMHLNnXjNJJKqJ2EF",
  "type": "message_batch",
  "processing_status": "in_progress",
  "request_counts": {
    "processing": 2,
    "succeeded": 0,
    "errored": 0,
    "canceled": 0,
    "expired": 0
  },
  "created_at": "2024-08-20T18:37:24.100435Z",
  "expires_at": "2024-08-21T18:37:24.100435Z",
  "ended_at": null
}
```

---

### 3.2 获取批处理状态 (Retrieve Message Batch)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/messages/batches/{message_batch_id}`
- **描述：** 查询批处理任务的状态和进度

**路径参数：**
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| message_batch_id | string | 是 | 批处理任务ID |

**响应字段：**
- `processing_status` - 处理状态："in_progress"|"succeeded"|"failed"
- `request_counts` - 各状态请求数量统计
- `results_url` - 结果下载URL
- `expires_at` - 过期时间（24小时后）

---

### 3.3 获取批处理结果 (Retrieve Batch Results)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/messages/batches/{message_batch_id}/results`
- **描述：** 获取批处理任务的详细结果

**响应格式（JSONL）：**
```jsonl
{"custom_id":"my-first-request","result":{"type":"succeeded","message":{...}}}
{"custom_id":"my-second-request","result":{"type":"errored","error":{...}}}
```

---

### 3.4 列出批处理任务 (List Message Batches)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/messages/batches`
- **描述：** 获取所有批处理任务的列表

**查询参数：**
- `before_id` - 游标分页
- `after_id` - 游标分页
- `limit` - 每页数量（1-1000，默认20）

---

### 3.5 取消批处理 (Cancel Message Batch)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/messages/batches/{message_batch_id}/cancel`
- **描述：** 取消正在处理的批处理任务

---

### 3.6 删除批处理 (Delete Message Batch)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `DELETE`
- **路径：** `/v1/messages/batches/{message_batch_id}`
- **描述：** 删除批处理任务及其结果

---

## 4. Files API

### 4.1 上传文件 (Upload File)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/files`
- **描述：** 上传文件以供API使用
- **Content-Type：** `multipart/form-data`

**请求参数：**
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| file | binary | 是 | 要上传的文件 |

**支持的文件类型：**
- PDF文档
- 图片（JPEG, PNG, GIF, WEBP）
- 文本文件

**响应格式：**
```json
{
  "id": "file_011CNha8iCJcU1wXNR6q4V8w",
  "type": "file",
  "filename": "document.pdf",
  "mime_type": "application/pdf",
  "size_bytes": 102400,
  "created_at": "2023-06-01T12:00:00Z",
  "downloadable": false
}
```

---

### 4.2 列出文件 (List Files)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/files`
- **描述：** 获取已上传文件的列表

**查询参数：**
- `before_id` - 游标分页
- `after_id` - 游标分页
- `limit` - 每页数量（1-1000，默认20）

---

### 4.3 获取文件元数据 (Get File Metadata)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/files/{file_id}`
- **描述：** 获取文件的元数据信息

---

### 4.4 获取文件内容 (Get File Content)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/files/{file_id}/content`
- **描述：** 下载文件的实际内容

---

### 4.5 删除文件 (Delete File)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `DELETE`
- **路径：** `/v1/files/{file_id}`
- **描述：** 删除已上传的文件

**响应格式：**
```json
{
  "id": "file_011CNha8iCJcU1wXNR6q4V8w",
  "type": "file_deleted"
}
```

---

## 5. Skills API

### 5.1 创建技能 (Create Skill)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/skills`
- **描述：** 创建一个新的技能

**请求头：**
```
anthropic-beta: skills-2025-10-02
```

---

### 5.2 列出技能 (List Skills)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/skills`
- **描述：** 获取所有可用技能的列表

**响应格式：**
```json
{
  "data": [
    {
      "id": "skill_01JAbcdefghijklmnopqrstuvw",
      "type": "skill",
      "display_title": "My Custom Skill",
      "source": "custom",
      "latest_version": "1759178010641129",
      "created_at": "2024-10-30T23:58:27.427722Z",
      "updated_at": "2024-10-30T23:58:27.427722Z"
    }
  ],
  "has_more": true,
  "next_page": "page_token_here"
}
```

---

### 5.3 获取技能详情 (Get Skill)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/skills/{skill_id}`
- **描述：** 获取指定技能的详细信息

---

### 5.4 删除技能 (Delete Skill)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `DELETE`
- **路径：** `/v1/skills/{skill_id}`
- **描述：** 删除指定的技能

**响应格式：**
```json
{
  "id": "skill_01JAbcdefghijklmnopqrstuvw",
  "type": "skill_deleted"
}
```

---

### 5.5 创建技能版本 (Create Skill Version)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/skills/{skill_id}/versions`
- **描述：** 为技能创建新版本

---

### 5.6 列出技能版本 (List Skill Versions)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/skills/{skill_id}/versions`
- **描述：** 获取技能的所有版本列表

---

### 5.7 获取技能版本 (Get Skill Version)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/skills/{skill_id}/versions/{version_id}`
- **描述：** 获取技能特定版本的详情

---

### 5.8 删除技能版本 (Delete Skill Version)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `DELETE`
- **路径：** `/v1/skills/{skill_id}/versions/{version_id}`
- **描述：** 删除技能的特定版本

---

## 6. Admin API

### 6.1 获取组织信息 (Get Organization)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `GET`
- **路径：** `/v1/organization`
- **描述：** 获取当前组织的详细信息

---

## 7. 实验性API

### 7.1 改进提示词 (Improve Prompt)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/experimental/improve_prompt`
- **描述：** 使用AI改进现有提示词

**请求头：**
```
anthropic-beta: prompt-tools-2025-04-02
```

**请求体参数：**
```json
{
  "messages": [
    {"role": "user", "content": [{"type": "text", "text": "原始提示词"}]}
  ],
  "system": "系统指令",
  "feedback": "改进建议",
  "target_model": "claude-3-7-sonnet-20250219"
}
```

---

### 7.2 生成提示词 (Generate Prompt)

**实现状态：** ❌ **未实现**

**接口信息：**
- **方法：** `POST`
- **路径：** `/v1/experimental/generate_prompt`
- **描述：** 根据描述生成新的提示词

---

## 8. 错误响应格式

所有API错误都遵循统一的响应格式：

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "错误描述"
  }
}
```

**错误类型：**
- `invalid_request_error` - 请求参数无效
- `authentication_error` - 认证失败
- `permission_error` - 权限不足
- `not_found_error` - 资源不存在
- `rate_limit_error` - 超出速率限制
- `api_error` - API内部错误
- `overloaded_error` - 服务过载
- `timeout_error` - 请求超时
- `billing_error` - 账单相关错误

---

## 9. 通用请求头

所有API请求都需要以下请求头：

```
x-api-key: <YOUR_API_KEY>
anthropic-version: 2023-06-01
content-type: application/json
```

某些Beta功能需要额外的请求头：

```
anthropic-beta: <beta_name>
```

例如：
- `files-api-2025-04-14` - Files API
- `skills-2025-10-02` - Skills API
- `prompt-tools-2025-04-02` - Prompt Tools

---

## 10. 实现优先级建议

基于当前项目需求，建议按以下优先级实现：

### 高优先级
1. ✅ Messages API - 已实现
2. ❌ Models API - 建议实现，用于动态获取可用模型列表

### 中优先级
3. ❌ Files API - 支持文档上传和处理
4. ❌ Batch API - 批量处理大规模请求

### 低优先级
5. ❌ Skills API - 高级功能，可选实现
6. ❌ Admin API - 组织管理功能
7. ❌ 实验性API - 提示词优化工具

---

## 11. 版本历史

- **2023-06-01** - 当前稳定版本
- **2025-04-02** - Prompt Tools Beta
- **2025-04-14** - Files API Beta
- **2025-10-02** - Skills API Beta

---

## 12. 相关资源

- [Claude官方文档](https://docs.anthropic.com/)
- [API版本管理](https://docs.anthropic.com/en/docs/build-with-claude/versioning)
- [错误处理指南](https://docs.anthropic.com/en/docs/build-with-claude/errors)
- [速率限制说明](https://docs.anthropic.com/en/docs/build-with-claude/rate-limits)

---

**文档最后更新：** 2025-11-20
**文档版本：** 1.0.0
