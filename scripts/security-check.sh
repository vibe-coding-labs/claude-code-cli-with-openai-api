#!/bin/bash
#
# Security Check Script for Open Source Release
# 检查项目是否安全可以开源
#

# Don't exit on error for grep commands
set +e

echo "🔒 开源安全检查开始..."
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查计数器
ERRORS=0
WARNINGS=0
PASSED=0

# 辅助函数
check_pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASSED++))
}

check_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
    ((WARNINGS++))
}

check_fail() {
    echo -e "${RED}✗${NC} $1"
    ((ERRORS++))
}

echo "=== 检查 1: .gitignore 配置 ==="

if [ -f .gitignore ]; then
    check_pass ".gitignore 文件存在"
    
    # 检查关键模式
    if grep -q "^\.env" .gitignore || grep -q "^.env" .gitignore; then
        check_pass ".env 已被忽略"
    else
        check_fail ".env 未在 .gitignore 中"
    fi
    
    if grep -q "\.db" .gitignore; then
        check_pass "*.db 文件已被忽略"
    else
        check_fail "*.db 文件未被忽略"
    fi
    
    if grep -q "\.log" .gitignore; then
        check_pass "*.log 文件已被忽略"
    else
        check_fail "*.log 文件未被忽略"
    fi
else
    check_fail ".gitignore 文件不存在"
fi

echo ""
echo "=== 检查 2: 敏感文件不在版本控制中 ==="

# 检查 .env 文件
if git ls-files | grep -q "^\.env$"; then
    check_fail ".env 文件已被 git 追踪！"
else
    check_pass ".env 文件未被追踪"
fi

# 检查数据库文件
if git ls-files | grep -q "\.db$"; then
    check_fail "发现 .db 文件被追踪"
else
    check_pass "没有 .db 文件被追踪"
fi

# 检查日志文件
if git ls-files | grep -q "\.log$"; then
    check_fail "发现 .log 文件被追踪"
else
    check_pass "没有 .log 文件被追踪"
fi

echo ""
echo "=== 检查 3: 搜索潜在的硬编码密钥 ==="

# 搜索可能的 OpenAI API Keys（跳过 node_modules 和示例文件）
if git grep -n "sk-[a-zA-Z0-9]\{20,\}" -- "*.go" "*.ts" "*.tsx" "*.js" "*.jsx" 2>/dev/null | grep -v "placeholder\|example\|示例\|sk-xxx\|sk-..."; then
    check_fail "发现可能的真实 API 密钥"
else
    check_pass "未发现硬编码的 API 密钥"
fi

# 检查密码相关（排除合法的密码字段定义）
SUSPICIOUS_PASSWORDS=$(git grep -in "password.*=.*['\"][^'\"]\{8,\}['\"]" -- "*.go" "*.ts" "*.tsx" "*.js" "*.jsx" 2>/dev/null | grep -v "placeholder\|example\|示例\|Password\"\|password:\|'password'\|\"password\"" || true)
if [ -n "$SUSPICIOUS_PASSWORDS" ]; then
    check_warn "发现可疑的密码赋值，请人工检查："
    echo "$SUSPICIOUS_PASSWORDS"
else
    check_pass "未发现可疑的硬编码密码"
fi

echo ""
echo "=== 检查 4: 敏感配置文件 ==="

# 检查本地是否存在敏感文件但未被追踪
if [ -f .env ]; then
    if git check-ignore .env > /dev/null; then
        check_pass ".env 存在但已被正确忽略"
    else
        check_fail ".env 存在但未被 gitignore"
    fi
fi

# 检查数据库文件
if ls data/*.db 2>/dev/null | grep -q .; then
    check_warn "data/ 目录下存在 .db 文件（确保已被 gitignore）"
else
    check_pass "未发现数据库文件"
fi

# 检查日志文件
if ls *.log 2>/dev/null | grep -q .; then
    check_warn "根目录下存在 .log 文件（确保已被 gitignore）"
else
    check_pass "未发现日志文件"
fi

echo ""
echo "=== 检查 5: Git 历史记录 ==="

# 检查 .env 是否曾被提交
if git log --all --full-history -- .env 2>/dev/null | grep -q .; then
    check_fail ".env 曾被提交到 git 历史！需要清理历史记录"
else
    check_pass ".env 从未被提交到 git"
fi

# 检查数据库文件历史
if git log --all --full-history -- "*.db" 2>/dev/null | grep -q .; then
    check_fail ".db 文件曾被提交！需要清理历史记录"
else
    check_pass ".db 文件从未被提交"
fi

echo ""
echo "=== 检查 6: 必需的文档文件 ==="

if [ -f LICENSE ]; then
    check_pass "LICENSE 文件存在"
else
    check_warn "LICENSE 文件不存在"
fi

if [ -f README.md ]; then
    check_pass "README.md 文件存在"
else
    check_fail "README.md 文件不存在"
fi

if [ -f SECURITY.md ]; then
    check_pass "SECURITY.md 文件存在"
else
    check_warn "SECURITY.md 文件不存在"
fi

if [ -f env.example ]; then
    check_pass "env.example 文件存在"
else
    check_warn "env.example 文件不存在"
fi

echo ""
echo "=== 检查 7: 部署文件检查 ==="

# 检查部署文件中的敏感信息
if [ -f deploy-prod.sh ]; then
    if git check-ignore deploy-prod.sh > /dev/null 2>&1; then
        check_pass "deploy-prod.sh 已被 gitignore 保护"
    else
        check_warn "deploy-prod.sh 包含服务器信息但未被忽略"
    fi
else
    check_pass "deploy-prod.sh 不存在或已使用模板"
fi

if [ -f k8s/deployment.yaml ]; then
    if git check-ignore k8s/deployment.yaml > /dev/null 2>&1; then
        check_pass "k8s/deployment.yaml 已被 gitignore 保护"
    else
        check_warn "k8s/deployment.yaml 包含配置但未被忽略"
    fi
fi

if [ -f deploy-prod.sh.template ]; then
    check_pass "部署模板文件已提供"
fi

echo ""
echo "=============================="
echo "=== 检查结果汇总 ==="
echo "=============================="
echo -e "${GREEN}通过: $PASSED${NC}"
echo -e "${YELLOW}警告: $WARNINGS${NC}"
echo -e "${RED}错误: $ERRORS${NC}"
echo ""

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}❌ 发现 $ERRORS 个错误！请先修复后再开源${NC}"
    exit 1
elif [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}⚠️  发现 $WARNINGS 个警告，建议检查后再开源${NC}"
    echo ""
    echo "建议操作："
    echo "1. 检查上述警告项"
    echo "2. 查看 .github/OPENSOURCE_CHECKLIST.md"
    echo "3. 阅读 SECURITY.md"
    exit 0
else
    echo -e "${GREEN}✅ 所有检查通过！项目可以安全开源${NC}"
    echo ""
    echo "下一步："
    echo "1. 审查 .github/OPENSOURCE_CHECKLIST.md"
    echo "2. 更新 README.md 中的项目信息"
    echo "3. 准备发布"
    exit 0
fi
