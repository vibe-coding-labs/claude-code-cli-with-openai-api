# Docker 快速发布指南

## 已准备的文件

| 文件 | 说明 |
|------|------|
| `Dockerfile` | 多平台构建版本（支持 amd64/arm64） |
| `Dockerfile.local` | 本地构建版本（当前平台） |
| `docker-publish.sh` | 一键发布脚本 |
| `DOCKER_HUB_GUIDE.md` | 完整发布指南 |
| `.github/workflows/docker-publish.yml` | GitHub Actions 自动发布 |

## 快速发布步骤

### 1. 命令行方式

```bash
# 登录 Docker Hub
docker login

# 构建并推送 (替换 your-username)
export DOCKER_HUB_USER=your-username
./docker-publish.sh v1.0.0
```

### 2. GitHub Actions 自动发布

1. 在 GitHub 仓库设置中添加 Secrets:
   - `DOCKER_HUB_USERNAME` - Docker Hub 用户名
   - `DOCKER_HUB_TOKEN` - Docker Hub 访问令牌

2. 推送标签自动触发构建:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

## 用户使用

```bash
# 拉取镜像
docker pull your-username/claude-with-openai-api:latest

# 运行
docker run -d \
  -p 54988:54988 \
  -v ./data:/app/data \
  -e OPENAI_API_KEY=sk-xxx \
  your-username/claude-with-openai-api:latest
```

## 注意事项

1. **构建问题**: 如果遇到网络问题，可以使用 Docker 镜像加速器
2. **多平台**: 本地构建使用 `Dockerfile.local`，CI/CD 使用 `Dockerfile`
3. **标签策略**: 建议使用语义化版本 (v1.0.0, v1.0, v1)
