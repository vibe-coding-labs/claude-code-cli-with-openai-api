# 根路径浏览器自动重定向功能

## 功能说明

当访问 `http://localhost:8083/` 时，系统会根据User-Agent自动判断：
- **浏览器访问**：自动重定向到 `/ui/`
- **API调用**：返回JSON格式的服务信息

## 实现细节

### 1. 浏览器检测逻辑

**检测为浏览器的User-Agent关键字：**
- Mozilla
- Chrome
- Safari
- Firefox
- Edge
- Opera
- MSIE
- Trident

**排除的API客户端关键字：**
- curl
- wget
- python
- go-http-client
- axios
- okhttp
- java
- apache-httpclient
- postman
- insomnia

### 2. 代码位置

文件：`handler/handler.go`

```go
// Root 处理根路径（别名）
// 如果是浏览器访问，重定向到 /ui/
// 如果是API调用，返回JSON
func (h *Handler) Root(c *gin.Context) {
    userAgent := c.GetHeader("User-Agent")
    
    // 检测是否是浏览器
    if isBrowserUserAgent(userAgent) {
        c.Redirect(http.StatusMovedPermanently, "/ui/")
        return
    }
    
    // 非浏览器访问，返回JSON
    h.Index(c)
}
```

## 测试方法

### 1. 浏览器测试
```bash
# 启动服务器
./claude-code-cli-with-openai-api server --port 8083

# 在浏览器中访问
open http://localhost:8083/
```

**预期结果：** 自动跳转到 `http://localhost:8083/ui/`

### 2. API客户端测试

**使用curl：**
```bash
curl http://localhost:8083/
```

**预期结果：**
```json
{
  "service": "claude-to-openai-proxy",
  "version": "1.0.0"
}
```

**使用curl模拟浏览器：**
```bash
curl -H "User-Agent: Mozilla/5.0" http://localhost:8083/
```

**预期结果：** 返回重定向HTML（301 Moved Permanently）

### 3. 各种User-Agent测试

```bash
# Chrome浏览器
curl -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36" http://localhost:8083/

# Safari浏览器
curl -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15" http://localhost:8083/

# Firefox浏览器
curl -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:120.0) Gecko/20100101 Firefox/120.0" http://localhost:8083/

# Python requests
curl -H "User-Agent: python-requests/2.31.0" http://localhost:8083/

# Postman
curl -H "User-Agent: PostmanRuntime/7.35.0" http://localhost:8083/
```

## 测试用例

| User-Agent | 预期行为 | 原因 |
|------------|---------|------|
| `Mozilla/5.0 (Chrome)` | 重定向到 `/ui/` | 包含"Mozilla"和"Chrome" |
| `Mozilla/5.0 (Safari)` | 重定向到 `/ui/` | 包含"Mozilla"和"Safari" |
| `curl/7.88.1` | 返回JSON | 包含"curl"（API客户端） |
| `python-requests/2.31.0` | 返回JSON | 包含"python"（API客户端） |
| `PostmanRuntime/7.35.0` | 返回JSON | 包含"postman"（API客户端） |
| （空User-Agent） | 返回JSON | 无User-Agent视为API调用 |

## 优势

1. **用户体验优化**：浏览器用户直接访问根路径即可进入UI界面
2. **API兼容性**：不影响API客户端的正常使用
3. **智能识别**：准确区分浏览器和API客户端
4. **SEO友好**：使用301永久重定向

## 注意事项

1. **重定向类型**：使用301（Moved Permanently）永久重定向
2. **API调用**：如果需要在浏览器中测试API，应直接访问具体的API端点
3. **开发工具**：Postman、Insomnia等API测试工具会正常返回JSON
4. **自定义User-Agent**：如果API客户端使用浏览器的User-Agent，会被重定向

## 修改历史

- **2025-11-20 01:08**：实现根路径浏览器自动重定向功能
- 修改文件：`handler/handler.go`
- 添加功能：`isBrowserUserAgent()` 和 `containsIgnoreCase()`

## 相关文件

- `handler/handler.go` - 主要实现
- `cmd/server.go` - 路由配置
