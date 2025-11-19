# 🚀 快速开始

## 已为你配置好iFlow API！

系统已经启动并运行在 **http://localhost:10086**

## 📊 管理界面

点击上方的浏览器预览按钮，或访问：

**http://localhost:10086/ui**

### 界面功能

1. **配置管理** - 创建和管理多个API配置
2. **详情页面** - 查看每个配置的详细统计
3. **Token统计** - 实时查看Token消耗
4. **在线测试** - 测试API配置是否正常

## 🎯 快速测试

### 1. 创建iFlow配置（推荐）

在管理界面点击"新建配置"，填写：

```
名称: iFlow API
API Key: sk-d35ba48d260a51054982b9a6794ca2d9
Base URL: https://apis.iflow.cn/v1
大模型: tstars2.0
中模型: qwen3-coder
小模型: qwen3-coder
```

### 2. 测试配置

创建后，点击"详情" → "在线测试"，系统会：
- 发送测试请求
- 显示响应时间
- 记录Token使用
- 更新统计数据

### 3. 查看统计

在详情页面查看：
- ✅ 总请求数
- 📊 成功率
- 🎯 Token消耗（输入/输出/总计）
- ⚡ 平均响应时间
- 📝 最近50条请求日志

## 🔧 使用Claude API

### 方法1: 直接调用

```bash
curl -X POST http://localhost:10086/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 100,
    "messages": [
      {
        "role": "user",
        "content": "你好，请介绍一下你自己"
      }
    ]
  }'
```

### 方法2: Claude Code CLI

```bash
export ANTHROPIC_BASE_URL=http://localhost:10086
export ANTHROPIC_API_KEY="any-value"
claude
```

## 🔐 数据安全

- ✅ API Key使用AES-256-GCM加密存储
- ✅ 界面只显示掩码后的Key（前8位+后4位）
- ✅ 数据库位置：`./data/proxy.db`

## 📈 查看Token消耗

所有请求会自动记录：

1. **按天统计**：在详情页"概览"标签查看
2. **详细日志**：在"请求日志"标签查看每个请求

统计包括：
- 输入Token数量
- 输出Token数量  
- 总Token消耗
- 请求耗时
- 成功/失败状态

## 🛠️ 管理多个API

你可以添加多个API配置：

1. **OpenAI官方** - 使用OpenAI的API
2. **iFlow平台** - 使用国内iFlow API
3. **其他兼容平台** - 任何OpenAI兼容的API

每个配置独立统计Token使用！

## 📁 文件说明

- `start-with-iflow.sh` - 快速启动脚本（已配置iFlow）
- `USAGE_GUIDE.md` - 完整使用手册
- `IFLOW_SETUP.md` - iFlow API配置说明
- `data/proxy.db` - SQLite数据库（自动创建）

## ❓ 常见问题

### Q: 如何停止服务器？

按 `Ctrl+C` 停止服务器

### Q: 如何修改端口？

编辑 `start-with-iflow.sh`，修改 `PORT` 变量

### Q: 忘记API Key怎么办？

在详情页点击"编辑"，输入新的API Key更新

### Q: Token统计多久更新一次？

每次请求都会实时更新统计数据

## 🎉 现在开始使用吧！

1. 打开管理界面：http://localhost:10086/ui
2. 创建或查看配置
3. 点击"在线测试"验证配置
4. 查看Token统计和请求日志

---

更多详细信息请参考 [完整使用手册](USAGE_GUIDE.md)
