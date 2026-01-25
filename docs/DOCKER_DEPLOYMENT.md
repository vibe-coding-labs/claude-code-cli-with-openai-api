# Docker 部署指南

## 概述

本指南介绍如何使用 Docker 和 Docker Compose 部署 Claude-to-OpenAI API 代理服务。

## 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- 至少 2GB 可用内存
- 至少 10GB 可用磁盘空间

## 快速开始

### 1. 构建镜像

```bash
# 构建镜像
docker build -t claude-proxy:latest .

# 查看镜像
docker images | grep claude-proxy
```

### 2. 运行容器

```bash
# 使用 docker run
docker run -d \
  --name claude-proxy \
  -p 54988:54988 \
  -v $(pwd)/data:/app/data \
  -e PORT=54988 \
  -e LOG_LEVEL=INFO \
  claude-proxy:latest

# 或使用 docker-compose
docker-compose up -d
```

### 3. 验证部署

```bash
# 检查容器状态
docker ps | grep claude-proxy

# 查看日志
docker logs claude-proxy

# 测试健康检查
curl http://localhost:54988/health

# 访问管理界面
open http://localhost:54988/ui
```

## Docker Compose 部署

### 开发环境

使用 `docker-compose.yml`：

```bash
# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down

# 重启服务
docker-compose restart
```

### 生产环境

使用 `docker-compose.prod.yml`：

```bash
# 设置环境变量
export DOCKER_REGISTRY=your-registry.com
export VERSION=v1.0.0
export GRAFANA_PASSWORD=your-secure-password

# 启动服务
docker-compose -f docker-compose.prod.yml up -d

# 查看服务状态
docker-compose -f docker-compose.prod.yml ps

# 查看日志
docker-compose -f docker-compose.prod.yml logs -f claude-proxy

# 停止服务
docker-compose -f docker-compose.prod.yml down
```

## 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| PORT | 54988 | 服务端口 |
| HOST | 0.0.0.0 | 监听地址 |
| LOG_LEVEL | INFO | 日志级别 |
| DB_PATH | /app/data/proxy.db | 数据库路径 |
| LB_HEALTH_CHECK_INTERVAL | 30 | 健康检查间隔（秒） |
| LB_FAILURE_THRESHOLD | 3 | 失败阈值 |
| LB_RECOVERY_THRESHOLD | 2 | 恢复阈值 |
| LB_MAX_RETRIES | 3 | 最大重试次数 |
| LB_CIRCUIT_BREAKER_ENABLED | true | 是否启用熔断器 |
| LB_ERROR_RATE_THRESHOLD | 0.5 | 错误率阈值 |

### 数据卷

```yaml
volumes:
  - ./data:/app/data          # 数据库文件
  - ./logs:/app/logs          # 日志文件
```

### 资源限制

```yaml
deploy:
  resources:
    limits:
      cpus: '2'               # CPU 限制
      memory: 1G              # 内存限制
    reservations:
      cpus: '1'               # CPU 预留
      memory: 512M            # 内存预留
```

## 健康检查

Docker 容器配置了健康检查：

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:54988/health || exit 1
```

查看健康状态：

```bash
# 查看容器健康状态
docker inspect --format='{{.State.Health.Status}}' claude-proxy

# 查看健康检查日志
docker inspect --format='{{json .State.Health}}' claude-proxy | jq
```

## 日志管理

### 查看日志

```bash
# 查看实时日志
docker logs -f claude-proxy

# 查看最近100行日志
docker logs --tail 100 claude-proxy

# 查看特定时间的日志
docker logs --since 1h claude-proxy
```

### 日志配置

生产环境配置了日志轮转：

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "100m"        # 单个日志文件最大100MB
    max-file: "10"          # 保留最多10个日志文件
```

## 数据备份

### 备份数据库

```bash
# 停止容器
docker-compose stop claude-proxy

# 备份数据库
cp data/proxy.db data/proxy.db.backup.$(date +%Y%m%d_%H%M%S)

# 启动容器
docker-compose start claude-proxy
```

### 在线备份

```bash
# 使用 SQLite 备份命令
docker exec claude-proxy sqlite3 /app/data/proxy.db ".backup /app/data/proxy.db.backup"

# 复制备份文件到宿主机
docker cp claude-proxy:/app/data/proxy.db.backup ./data/
```

## 升级和回滚

### 升级流程

```bash
# 1. 备份数据
docker-compose stop
cp -r data data.backup

# 2. 拉取新镜像
docker pull your-registry.com/claude-proxy:v1.1.0

# 3. 更新配置
export VERSION=v1.1.0

# 4. 启动新版本
docker-compose -f docker-compose.prod.yml up -d

# 5. 验证服务
curl http://localhost:54988/health
docker logs claude-proxy
```

### 回滚流程

```bash
# 1. 停止服务
docker-compose -f docker-compose.prod.yml down

# 2. 恢复数据
rm -rf data
mv data.backup data

# 3. 启动旧版本
export VERSION=v1.0.0
docker-compose -f docker-compose.prod.yml up -d

# 4. 验证服务
curl http://localhost:54988/health
```

### 零停机升级

使用滚动更新：

```bash
# 配置滚动更新策略
docker-compose -f docker-compose.prod.yml up -d --scale claude-proxy=2

# Docker Swarm 模式
docker service update \
  --image your-registry.com/claude-proxy:v1.1.0 \
  --update-parallelism 1 \
  --update-delay 10s \
  claude-proxy
```

## 监控集成

### Prometheus

生产环境配置包含 Prometheus：

```bash
# 访问 Prometheus
open http://localhost:9090

# 查看指标
curl http://localhost:54988/metrics
```

### Grafana

生产环境配置包含 Grafana：

```bash
# 访问 Grafana
open http://localhost:3000

# 默认用户名: admin
# 默认密码: 通过环境变量 GRAFANA_PASSWORD 设置
```

配置 Grafana 数据源：

1. 登录 Grafana
2. 添加数据源 → Prometheus
3. URL: http://prometheus:9090
4. 保存并测试

## 故障排查

### 容器无法启动

```bash
# 查看容器日志
docker logs claude-proxy

# 查看容器详细信息
docker inspect claude-proxy

# 检查端口占用
netstat -tlnp | grep 54988
```

### 健康检查失败

```bash
# 进入容器
docker exec -it claude-proxy sh

# 测试健康检查端点
wget -O- http://localhost:54988/health

# 检查进程
ps aux | grep claude
```

### 数据库锁定

```bash
# 检查数据库连接
docker exec claude-proxy sqlite3 /app/data/proxy.db "PRAGMA busy_timeout"

# 重启容器
docker-compose restart claude-proxy
```

### 内存不足

```bash
# 查看容器资源使用
docker stats claude-proxy

# 增加内存限制
# 编辑 docker-compose.yml
deploy:
  resources:
    limits:
      memory: 2G
```

## 安全最佳实践

### 1. 使用非 root 用户

Dockerfile 已配置使用非 root 用户：

```dockerfile
USER app
```

### 2. 限制资源

配置资源限制防止资源耗尽：

```yaml
deploy:
  resources:
    limits:
      cpus: '2'
      memory: 1G
```

### 3. 网络隔离

使用自定义网络隔离容器：

```yaml
networks:
  claude-network:
    driver: bridge
```

### 4. 敏感信息管理

使用 Docker Secrets 或环境变量文件：

```bash
# 创建 .env 文件
echo "OPENAI_API_KEY=your-key" > .env

# 使用 env_file
docker-compose --env-file .env up -d
```

### 5. 镜像扫描

定期扫描镜像漏洞：

```bash
# 使用 Docker Scout
docker scout cves claude-proxy:latest

# 使用 Trivy
trivy image claude-proxy:latest
```

## 性能优化

### 1. 构建优化

使用多阶段构建减小镜像大小：

```dockerfile
FROM golang:1.24-alpine AS backend-builder
# ... 构建步骤

FROM alpine:latest
# ... 只复制必要文件
```

### 2. 缓存优化

利用 Docker 层缓存：

```dockerfile
# 先复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 再复制源代码
COPY . .
```

### 3. 资源调优

根据负载调整资源：

```yaml
deploy:
  resources:
    limits:
      cpus: '4'      # 高负载场景
      memory: 2G
```

### 4. 连接池配置

通过环境变量配置连接池：

```yaml
environment:
  - DB_MAX_OPEN_CONNS=25
  - DB_MAX_IDLE_CONNS=5
```

## 生产环境检查清单

部署前检查：

- [ ] 数据库已备份
- [ ] 环境变量已配置
- [ ] 资源限制已设置
- [ ] 健康检查已配置
- [ ] 日志轮转已配置
- [ ] 监控已集成
- [ ] 告警已配置
- [ ] 回滚方案已准备

部署后验证：

- [ ] 容器状态正常
- [ ] 健康检查通过
- [ ] API 端点可访问
- [ ] 管理界面可访问
- [ ] 日志正常输出
- [ ] 监控指标正常
- [ ] 数据库连接正常
- [ ] 负载均衡器工作正常

## 参考资料

- [Docker 官方文档](https://docs.docker.com/)
- [Docker Compose 文档](https://docs.docker.com/compose/)
- [Docker 最佳实践](https://docs.docker.com/develop/dev-best-practices/)
- [容器安全最佳实践](https://docs.docker.com/engine/security/)

## 附录

### 常用命令

```bash
# 构建镜像
docker build -t claude-proxy:latest .

# 运行容器
docker run -d --name claude-proxy -p 54988:54988 claude-proxy:latest

# 查看日志
docker logs -f claude-proxy

# 进入容器
docker exec -it claude-proxy sh

# 停止容器
docker stop claude-proxy

# 删除容器
docker rm claude-proxy

# 查看资源使用
docker stats claude-proxy

# 导出镜像
docker save claude-proxy:latest | gzip > claude-proxy.tar.gz

# 导入镜像
docker load < claude-proxy.tar.gz
```

### Makefile 示例

```makefile
.PHONY: build run stop logs clean

IMAGE_NAME := claude-proxy
VERSION := latest

build:
	docker build -t $(IMAGE_NAME):$(VERSION) .

run:
	docker-compose up -d

stop:
	docker-compose down

logs:
	docker-compose logs -f

clean:
	docker-compose down -v
	docker rmi $(IMAGE_NAME):$(VERSION)

prod-deploy:
	docker-compose -f docker-compose.prod.yml up -d

prod-stop:
	docker-compose -f docker-compose.prod.yml down
```
