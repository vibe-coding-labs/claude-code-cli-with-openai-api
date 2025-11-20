# 端口配置说明

## ⚠️ 重要警告

**严禁随意修改端口！前后端端口配置需要保持一致！**

## 固定端口配置

| 服务 | 端口 | 说明 |
|------|------|------|
| 后端 API | **54988** | 后端服务器监听端口 |
| 前端 UI | **54989** | 前端开发服务器端口 |

## 访问地址

- **前端界面**: http://localhost:54989/ui
- **后端API**: http://localhost:54988/api
- **健康检查**: http://localhost:54988/health

## 配置文件位置

### 后端端口配置

1. **环境变量文件** (`.env`)
   ```bash
   PORT=54988
   ```

2. **默认配置** (`config/config.go`)
   ```go
   // ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
   Port: getEnvAsInt("PORT", 54988),
   ```

3. **命令行参数** (`cmd/server.go`, `cmd/ui.go`)
   ```go
   // ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
   serverCmd.Flags().IntVarP(&port, "port", "p", 54988, "Server port...")
   ```

### 前端端口配置

1. **package.json**
   ```json
   {
     "scripts": {
       "start": "PORT=54989 react-scripts start"
     }
   }
   ```

2. **axios配置** (`frontend/src/index.tsx`)
   ```typescript
   // ⚠️ 严禁随意修改端口！前后端端口配置需要保持一致！
   // 后端固定端口：54988，前端固定端口：54989
   axios.defaults.baseURL = 'http://localhost:54988';
   ```

3. **API服务配置** (`frontend/src/services/api.ts`)
   ```typescript
   // ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
   const API_BASE_URL = 'http://localhost:54988';
   ```

4. **认证服务配置** (`frontend/src/services/auth.ts`)
   ```typescript
   // ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
   const API_BASE_URL = 'http://localhost:54988/api';
   ```

## 启动服务

### 启动后端
```bash
cd /path/to/project
./claude-code-cli-with-openai-api server
# 服务将在 http://0.0.0.0:54988 启动
```

### 启动前端
```bash
cd frontend
npm start
# 服务将在 http://localhost:54989 启动
```

## 修改端口的影响

如果必须修改端口（**强烈不建议**），需要同时修改以下所有位置：

### 后端 (修改为 NEW_BACKEND_PORT)
- [ ] `env.example` - PORT=NEW_BACKEND_PORT
- [ ] `config/config.go` - 默认端口
- [ ] `cmd/server.go` - 默认端口
- [ ] `cmd/ui.go` - 默认端口

### 前端 (修改为 NEW_FRONTEND_PORT)
- [ ] `frontend/package.json` - PORT=NEW_FRONTEND_PORT
- [ ] `frontend/src/index.tsx` - axios.defaults.baseURL
- [ ] `frontend/src/services/api.ts` - API_BASE_URL
- [ ] `frontend/src/services/auth.ts` - API_BASE_URL

### 测试清单
- [ ] 后端服务成功启动在新端口
- [ ] 前端服务成功启动在新端口
- [ ] 登录功能正常
- [ ] API请求正常（无404错误）
- [ ] 配置列表正常显示
- [ ] 日志功能正常

## 常见问题

### Q: 为什么不能使用3000/8083端口？
A: 这两个端口可能与系统其他服务冲突。54988/54989是经过协调的固定端口。

### Q: 修改端口后前端无法访问后端API怎么办？
A: 检查前端所有axios配置是否都指向了新的后端端口，并清除浏览器缓存后重试。

### Q: 如何验证端口配置是否正确？
A: 运行以下命令检查端口监听状态：
```bash
lsof -i :54988,54989
```

## 技术支持

如遇端口配置问题，请检查：
1. 端口是否被其他进程占用
2. 防火墙是否允许这些端口
3. 浏览器缓存是否已清除
4. 前后端服务是否都已重启
