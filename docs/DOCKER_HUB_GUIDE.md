# Docker Hub 发布指南

## 快速开始

### 1. 登录 Docker Hub

```bash
docker login
# 输入您的 Docker Hub 用户名和密码
```

### 2. 构建镜像

#### 方式一：本地构建（推荐）

```bash
# 设置您的 Docker Hub 用户名
export DOCKER_HUB_USER=your-username

# 构建镜像
docker build -f Dockerfile.local -t claude-with-openai-api:latest .

# 标记镜像
docker tag claude-with-openai-api:latest ${DOCKER_HUB_USER}/claude-with-openai-api:latest
docker tag claude-with-openai-api:latest ${DOCKER_HUB_USER}/claude-with-openai-api:v1.0.0

# 推送镜像
docker push ${DOCKER_HUB_USER}/claude-with-openai-api:latest
docker push ${DOCKER_HUB_USER}/claude-with-openai-api:v1.0.0
```

#### 方式二：使用脚本

```bash
# 设置环境变量
export DOCKER_HUB_USER=your-username

# 使用脚本构建并推送
./docker-publish.sh v1.0.0
```

#### 方式三：多平台构建 (支持 amd64 和 arm64)

```bash
# 创建 buildx 构建器
docker buildx create --name mybuilder --use
docker buildx inspect --bootstrap

# 构建并推送多平台镜像
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ${DOCKER_HUB_USER}/claude-with-openai-api:latest \
  -t ${DOCKER_HUB_USER}/claude-with-openai-api:v1.0.0 \
  --push .
```

### 3. 验证推送

```bash
# 查看已推送的镜像
docker pull ${DOCKER_HUB_USER}/claude-with-openai-api:latest
docker images | grep claude-with-openai-api
```

## 使用镜像

### Docker 运行

```bash
# 拉取镜像
docker pull your-username/claude-with-openai-api:latest

# 运行容器
docker run -d \
  --name claude-proxy \
  -p 54988:54988 \
  -v $(pwd)/data:/app/data \
  -e OPENAI_API_KEY=your-api-key \
  your-username/claude-with-openai-api:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  claude-proxy:
    image: your-username/claude-with-openai-api:latest
    container_name: claude-proxy
    ports:
      - "54988:54988"
    volumes:
      - ./data:/app/data
      - ./logs:/app/logs
    environment:
      - PORT=54988
      - HOST=0.0.0.0
      - LOG_LEVEL=INFO
      - DB_PATH=/app/data/proxy.db
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_BASE_URL=${OPENAI_BASE_URL:-https://api.openai.com/v1}
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:54988/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

## 环境变量配置

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `PORT` | 54988 | 服务端口 |
| `HOST` | 0.0.0.0 | 监听地址 |
| `LOG_LEVEL` | INFO | 日志级别 |
| `DB_PATH` | /app/data/proxy.db | 数据库路径 |
| `OPENAI_API_KEY` | - | OpenAI API Key |
| `OPENAI_BASE_URL` | https://api.openai.com/v1 | OpenAI API 地址 |
| `BIG_MODEL` | gpt-4o | 大模型 |
| `MIDDLE_MODEL` | gpt-4o | 中模型 |
| `SMALL_MODEL` | gpt-4o-mini | 小模型 |

## 数据持久化

容器内的数据目录为 `/app/data`，建议挂载到宿主机：

```bash
-v $(pwd)/data:/app/data
```

## 故障排查

### 1. 容器无法启动

```bash
# 查看日志
docker logs claude-proxy

# 检查端口占用
lsof -i :54988
```

### 2. 数据库权限问题

```bash
# 确保数据目录有正确权限
mkdir -p data
chmod 755 data
```

### 3. 网络连接问题

```bash
# 进入容器测试网络
docker exec -it claude-proxy sh
wget --spider http://localhost:54988/health
```

## GitHub Actions 自动发布

可以配置 GitHub Actions 在每次发布时自动构建并推送到 Docker Hub。

参考配置见 `.github/workflows/docker-publish.yml`
