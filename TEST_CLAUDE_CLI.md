# Claude Code CLI 测试指南

## 问题总结

当前遇到的问题：
1. ✅ 数据库和加密功能正常
2. ✅ 配置创建成功
3. ❌ 路由/proxy/:id/v1/messages 返回404

## 路由配置

在`cmd/server.go`中配置：
```go
proxyGroup := router.Group("/proxy/:id/v1")
{
    proxyGroup.POST("/messages", h.CreateMessageWithConfig)
    proxyGroup.POST("/messages/count_tokens", h.CountTokens)
}
```

应该匹配: `/proxy/ff40e638-918a-4556-b3c5-4155d1cc4156/v1/messages`

## 调试步骤

1. 检查Gin是否正确注册路由
2. 添加调试日志查看路由匹配过程
3. 测试简化版本的路由（例如：`/test/:id`）

## 下一步

需要调试为什么Gin路由没有匹配到`/proxy/:id/v1/messages`这个模式。
