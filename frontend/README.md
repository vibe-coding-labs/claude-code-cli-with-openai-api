# Claude-to-OpenAI API Proxy - Frontend

React + TypeScript + Ant Design前端界面，用于管理多个OpenAI API配置。

## 开发

```bash
# 安装依赖
npm install

# 启动开发服务器
npm start
```

开发服务器将在 http://localhost:3000 启动。

## 构建

```bash
# 构建生产版本
npm run build
```

构建产物将在 `build` 目录中。

## 使用

1. 构建前端：
   ```bash
   cd frontend
   npm install
   npm run build
   ```

2. 启动服务器（带Web界面）：
   ```bash
   cd ..
   ./claude-with-openai-api ui
   ```

3. 访问Web界面：
   - 打开浏览器访问 http://localhost:10086/ui
   - OpenAI API配置：创建、编辑、删除和管理多个OpenAI API配置
   - API文档：查看API使用文档

## 功能

- ✅ OpenAI API配置管理（创建、编辑、删除）
- ✅ 配置测试
- ✅ 多配置支持
- ✅ 默认配置设置
- ✅ Claude格式配置导出
- ✅ API文档查看

## 技术栈

- React 19
- TypeScript
- Ant Design 5
- React Router
- Axios
