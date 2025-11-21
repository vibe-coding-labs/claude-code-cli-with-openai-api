# Open Source Preparation Checklist

在开源此项目前，请确认以下事项：

## ✅ 已完成的安全检查

- [x] `.gitignore` 已正确配置
- [x] 没有硬编码的真实 API 密钥
- [x] `.env` 文件已被忽略且未提交到 git
- [x] 数据库文件 (`*.db`) 已被忽略
- [x] 日志文件 (`*.log`) 已被忽略
- [x] 二进制文件已被忽略
- [x] `env.example` 只包含示例值

## 🔍 需要决策的事项

### 1. 部署文件处理

**当前状态**：
- `deploy-prod.sh` 包含服务器 IP (8.130.35.126) 和域名
- `k8s/deployment.yaml` 和 `k8s/ingress.yaml` 包含命名空间和域名

**选项**：
- [ ] **选项A**：保持不变（推荐 - 这些是基础设施信息，不是密钥）
- [ ] **选项B**：将这些文件改为模板，添加 `.gitignore`：
  ```bash
  deploy-prod.sh
  k8s/deployment.yaml
  k8s/ingress.yaml
  ```
  并创建：
  ```
  deploy-prod.sh.template
  k8s/deployment.yaml.template
  k8s/ingress.yaml.template
  ```
- [ ] **选项C**：使用环境变量替换硬编码值

### 2. 加密密钥

**当前状态**：
- `database/encryption.go` 使用固定的加密密钥（有意设计）
- 已添加详细注释说明设计权衡

**建议**：
- [x] 在 `SECURITY.md` 中说明此设计决策
- [ ] （可选）考虑添加环境变量支持，允许用户自定义密钥

## 📝 推荐添加的文件

### 必需文件

- [x] `LICENSE` - ✅ 已存在
- [x] `README.md` - ✅ 已存在
- [x] `SECURITY.md` - ✅ 已创建
- [ ] `CONTRIBUTING.md` - 贡献指南
- [ ] `CODE_OF_CONDUCT.md` - 行为准则

### 可选但推荐

- [ ] `.github/ISSUE_TEMPLATE/` - Issue 模板
- [ ] `.github/PULL_REQUEST_TEMPLATE.md` - PR 模板
- [ ] `CHANGELOG.md` - 变更日志

## 🔐 最终检查

在公开仓库前，请执行：

```bash
# 1. 检查是否有未追踪的敏感文件
git status

# 2. 检查历史记录中是否有敏感信息
git log --all --full-history --oneline

# 3. 搜索可能的敏感信息
git grep -i "password"
git grep -i "secret"
git grep -E "sk-[a-zA-Z0-9]{20,}"

# 4. 确认所有敏感文件都被忽略
git check-ignore .env data/*.db *.log

# 5. 查看将要提交的文件
git ls-files
```

## 📋 发布前准备

- [ ] 更新 `README.md`，添加：
  - [ ] 项目简介和特性
  - [ ] 安装说明
  - [ ] 使用文档
  - [ ] 配置指南
  - [ ] 常见问题
  - [ ] 贡献指南链接
  
- [ ] 确认许可证信息正确
- [ ] 添加徽章（build status, license, version 等）
- [ ] 准备示例配置和截图
- [ ] 编写详细的文档
- [ ] 测试全新安装流程

## 🚀 开源后的维护

- [ ] 设置 GitHub Actions 进行 CI/CD
- [ ] 配置 dependabot 自动更新依赖
- [ ] 监控 security alerts
- [ ] 准备回应 issues 和 PRs
- [ ] 定期更新文档

## ⚠️ 特别注意

### 文件不应出现在公共仓库：

1. `.env` - 本地环境配置
2. `data/*.db` - 运行时数据库
3. `*.log` - 日志文件
4. 编译后的二进制文件
5. `node_modules/` - 依赖包
6. IDE 配置文件（已在 .gitignore）

### 可以安全开源的文件：

1. `env.example` - 配置模板
2. `deploy-prod.sh` - 部署脚本（如果你决定保留）
3. `k8s/*.yaml` - Kubernetes 配置（如果你决定保留）
4. 所有源代码文件
5. 文档和配置文件

## 📞 联系方式

如果发现任何安全问题，请参考 `SECURITY.md` 中的报告流程。
