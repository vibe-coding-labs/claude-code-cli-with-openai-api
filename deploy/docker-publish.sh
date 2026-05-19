#!/bin/bash
# Docker Hub 发布脚本
# 用法: ./docker-publish.sh [版本号]

set -e

# 配置
DOCKER_HUB_USER=${DOCKER_HUB_USER:-"your-username"}
IMAGE_NAME="claude-with-openai-api"
VERSION=${1:-"latest"}

# 完整的镜像名
FULL_IMAGE_NAME="${DOCKER_HUB_USER}/${IMAGE_NAME}:${VERSION}"

echo "=========================================="
echo "Docker Hub 发布脚本"
echo "=========================================="
echo ""
echo "镜像名: ${FULL_IMAGE_NAME}"
echo ""

# 检查 Docker 登录状态
if ! docker info 2>/dev/null | grep -q "Username"; then
    echo "⚠️  未登录 Docker Hub，请先登录:"
    echo "   docker login"
    exit 1
fi

echo "✅ 已登录 Docker Hub"
echo ""

# 构建镜像 (当前平台)
echo "🔨 开始构建镜像..."
docker build -t ${IMAGE_NAME}:${VERSION} .

# 标记镜像
echo "🏷️  标记镜像..."
docker tag ${IMAGE_NAME}:${VERSION} ${FULL_IMAGE_NAME}

# 推送到 Docker Hub
echo "📤 推送到 Docker Hub..."
docker push ${FULL_IMAGE_NAME}

echo ""
echo "=========================================="
echo "✅ 发布成功!"
echo "=========================================="
echo ""
echo "拉取命令:"
echo "  docker pull ${FULL_IMAGE_NAME}"
echo ""
echo "运行命令:"
echo "  docker run -d -p 54988:54988 -v ./data:/app/data ${FULL_IMAGE_NAME}"
echo ""
