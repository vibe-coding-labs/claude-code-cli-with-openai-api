#!/bin/bash

# 启动Claude-to-OpenAI代理服务器，默认配置使用iFlow API
# 使用方法: ./start-with-iflow.sh

echo "============================================"
echo "Claude-to-OpenAI API Proxy Server"
echo "使用iFlow API配置启动"
echo "============================================"
echo ""

# 设置环境变量
export OPENAI_API_KEY="sk-d35ba48d260a51054982b9a6794ca2d9"
export OPENAI_BASE_URL="https://apis.iflow.cn/v1"
export BIG_MODEL="tstars2.0"
export MIDDLE_MODEL="qwen3-coder"
export SMALL_MODEL="qwen3-coder"
export PORT=10086
export LOG_LEVEL="INFO"
export ANTHROPIC_API_KEY=""  # 禁用客户端API key验证
export DB_PATH="./data/proxy.db"
export ENCRYPTION_KEY="iflow-proxy-secret-key-change-me"

echo "配置信息:"
echo "  Base URL: ${OPENAI_BASE_URL}"
echo "  Big Model: ${BIG_MODEL}"
echo "  Middle Model: ${MIDDLE_MODEL}"
echo "  Small Model: ${SMALL_MODEL}"
echo "  Port: ${PORT}"
echo "  Database: ${DB_PATH}"
echo ""
echo "启动服务器..."
echo ""

# 启动服务器
./claude-with-openai-api server

echo ""
echo "服务器已停止"
