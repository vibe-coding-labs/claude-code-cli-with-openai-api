# 部署指南

本文档说明如何部署此项目到生产环境。

## 前置要求

- Docker 和 Docker Compose
- Kubernetes 集群（可选）
- 服务器 SSH 访问权限

## 快速部署

### 1. 配置环境变量

从模板创建你自己的部署脚本：

```bash
# 复制模板文件
cp deploy-prod.sh.template deploy-prod.sh
cp k8s/deployment.yaml.template k8s/deployment.yaml
cp k8s/ingress.yaml.template k8s/ingress.yaml
```

### 2. 设置部署变量

编辑 `deploy-prod.sh`，替换以下环境变量：

```bash
# Docker 镜像仓库地址
DOCKER_REGISTRY="your-docker-registry.example.com"

# Kubernetes 命名空间
K8S_NAMESPACE="your-namespace"

# 部署服务器（格式：user@ip）
DEPLOY_SERVER="user@your-server-ip"
```

### 3. 配置 Kubernetes

编辑 `k8s/deployment.yaml` 和 `k8s/ingress.yaml`，替换：

- `${K8S_NAMESPACE}` - 你的 Kubernetes 命名空间
- `${DOCKER_REGISTRY}` - 你的 Docker 镜像仓库
- `${YOUR_DOMAIN}` - 你的域名
- `${TLS_SECRET_NAME}` - TLS 证书 Secret 名称

### 4. 执行部署

```bash
chmod +x deploy-prod.sh
./deploy-prod.sh
```

## Docker 部署（简单方式）

如果不使用 Kubernetes，可以直接用 Docker 运行：

```bash
# 构建镜像
docker build -t claude-code-cli-openai-api .

# 运行容器
docker run -d \
  --name claude-code-cli-openai-api \
  -p 54988:54988 \
  -v $(pwd)/data:/app/data \
  -e OPENAI_API_KEY=your-key \
  -e PORT=54988 \
  claude-code-cli-openai-api
```

## 环境变量配置

创建 `.env` 文件（参考 `env.example`）：

```bash
# 必需配置
OPENAI_API_KEY=sk-your-openai-api-key

# 可选配置
OPENAI_BASE_URL=https://api.openai.com/v1
BIG_MODEL=gpt-4o
MIDDLE_MODEL=gpt-4o
SMALL_MODEL=gpt-4o-mini
PORT=54988
HOST=0.0.0.0
```

## 安全建议

### 1. SSH 密钥配置

确保部署服务器已配置 SSH 密钥认证：

```bash
# 生成密钥（如果还没有）
ssh-keygen -t rsa -b 4096

# 将公钥复制到服务器
ssh-copy-id user@your-server
```

### 2. Docker Registry 认证

登录到你的私有镜像仓库：

```bash
docker login your-registry.example.com
```

### 3. Kubernetes 配置

确保 `~/.kube/config` 已正确配置集群访问权限。

### 4. TLS/SSL 证书

为生产环境配置 HTTPS：

```bash
# 使用 cert-manager 自动管理证书（推荐）
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# 或者手动创建证书 Secret
kubectl create secret tls your-tls-secret \
  --cert=path/to/cert.pem \
  --key=path/to/key.pem \
  -n your-namespace
```

## 监控和日志

### 查看应用日志

```bash
# Docker
docker logs -f claude-code-cli-openai-api

# Kubernetes
kubectl logs -f deployment/claude-code-cli-openai-api -n your-namespace
```

### 健康检查

```bash
curl http://your-domain/health
```

### 查看应用状态

```bash
# Docker
docker ps | grep claude-code-cli-openai-api

# Kubernetes
kubectl get pods -n your-namespace
kubectl get deployment -n your-namespace
```

## 回滚

如果部署出现问题，可以快速回滚：

```bash
# Kubernetes 回滚到上一个版本
kubectl rollout undo deployment/claude-code-cli-openai-api -n your-namespace

# 查看回滚历史
kubectl rollout history deployment/claude-code-cli-openai-api -n your-namespace
```

## 故障排查

### 1. 容器无法启动

```bash
# 检查日志
docker logs claude-code-cli-openai-api

# 检查环境变量
docker inspect claude-code-cli-openai-api | grep -A 20 Env
```

### 2. 无法访问服务

```bash
# 检查端口映射
docker port claude-code-cli-openai-api

# 检查防火墙规则
sudo ufw status
```

### 3. Kubernetes Pod 不正常

```bash
# 查看 Pod 详情
kubectl describe pod <pod-name> -n your-namespace

# 查看事件
kubectl get events -n your-namespace --sort-by='.lastTimestamp'
```

## 更新部署

```bash
# 1. 拉取最新代码
git pull

# 2. 重新构建并推送镜像
docker build -t your-registry/claude-code-cli-openai-api:latest .
docker push your-registry/claude-code-cli-openai-api:latest

# 3. 重启 Kubernetes 部署
kubectl rollout restart deployment/claude-code-cli-openai-api -n your-namespace

# 4. 等待部署完成
kubectl rollout status deployment/claude-code-cli-openai-api -n your-namespace
```

## 数据备份

定期备份 SQLite 数据库：

```bash
# 创建备份
sqlite3 data/proxy.db ".backup data/proxy.db.backup-$(date +%Y%m%d)"

# 或使用 cron 定时备份
0 2 * * * sqlite3 /path/to/data/proxy.db ".backup /path/to/backups/proxy.db.backup-$(date +\%Y\%m\%d)"
```

## 生产环境清单

部署到生产前，请确认：

- [ ] 已配置正确的环境变量
- [ ] 已设置 HTTPS/TLS
- [ ] 已配置防火墙规则
- [ ] 已设置日志轮转
- [ ] 已配置监控和告警
- [ ] 已测试健康检查端点
- [ ] 已配置自动备份
- [ ] 已准备回滚方案
- [ ] 已设置资源限制（CPU/内存）

## 支持

如遇部署问题，请查看：
- [项目 README](README.md)
- [安全指南](SECURITY.md)
- [GitHub Issues](https://github.com/your-repo/issues)
