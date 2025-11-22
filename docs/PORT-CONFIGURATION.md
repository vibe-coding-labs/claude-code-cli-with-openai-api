# 端口配置说明

## 📌 端口规划（严禁随意修改！）

根据代码中的注释：**⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989**

### 端口分配

| 端口 | 用途 | 使用场景 |
|------|------|----------|
| **54988** | 后端服务器 | 生产环境和开发环境 |
| **54989** | 前端开发服务器 | 仅开发时使用（npm start） |

## 🎯 正确的使用方式

### 生产环境（推荐）

使用嵌入式前端，只需要运行后端：

```bash
# 编译前端（打包到 build 目录）
cd frontend
npm run build

# 编译后端（嵌入前端文件）
cd ..
go build -o claude-with-openai-api

# 运行后端服务器（默认 54988 端口）
./claude-with-openai-api server

# 访问应用
http://localhost:54988/ui
```

**重要**: 不要使用 `PORT=54989`！默认就是 54988！

### 开发环境

需要同时运行前端开发服务器和后端：

```bash
# 终端1: 运行后端（54988端口）
./claude-with-openai-api server

# 终端2: 运行前端开发服务器（54989端口）
cd frontend
npm start
# 这会自动打开 http://localhost:54989

# 前端开发服务器会代理 API 请求到后端的 54988 端口
```

## ❌ 常见错误

### 错误 1: 混淆端口
```bash
# ❌ 错误：用 54989 启动后端
PORT=54989 ./claude-with-openai-api server

# ✅ 正确：使用默认的 54988
./claude-with-openai-api server
```

### 错误 2: CORS 配置
后端的 CORS 配置允许：
- `http://localhost:54989` （前端开发服务器）
- `http://127.0.0.1:54989` （前端开发服务器）

如果后端运行在 54989，会导致 CORS 错误！

## 🔧 架构说明

### 生产模式（嵌入式前端）
```
用户浏览器
    ↓
http://localhost:54988/ui
    ↓
后端服务器 (54988)
    ├── 提供静态文件 (/ui/*)
    └── 提供 API (/api/*)
```

### 开发模式
```
用户浏览器
    ↓
http://localhost:54989
    ↓
前端开发服务器 (54989)
    ├── 提供前端页面
    └── 代理 API 请求
        ↓
    后端服务器 (54988)
        └── 提供 API (/api/*)
```

## 📝 配置文件位置

1. **后端默认端口**: `config/config.go` 第 47 行
   ```go
   Port: getEnvAsInt("PORT", 54988),
   ```

2. **前端开发端口**: `frontend/package.json` 第 29 行
   ```json
   "start": "PORT=54989 react-scripts start"
   ```

3. **CORS 白名单**: `cmd/server.go` 第 145 行
   ```go
   if origin == "http://localhost:54989" || origin == "http://127.0.0.1:54989" {
   ```

## 🚀 快速检查

运行此命令检查端口占用：

```bash
# 检查 54988 端口
lsof -i :54988

# 检查 54989 端口
lsof -i :54989
```

## 📊 总结

- **54988**: 后端服务器的固定端口（生产 + 开发）
- **54989**: 前端开发服务器的固定端口（仅开发）
- **生产部署**: 只运行后端在 54988，访问 `http://localhost:54988/ui`
- **本地开发**: 后端 54988 + 前端 54989，访问 `http://localhost:54989`
