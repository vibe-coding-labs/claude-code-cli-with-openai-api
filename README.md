# Claude Code CLI with OpenAI API

A Go-based proxy server that enables **Claude Code CLI** to work with OpenAI-compatible API providers. This is a Golang port of the [claude-code-proxy](https://github.com/fuergaosi233/claude-code-proxy) project.

## Features

- **Full Claude API Compatibility**: Complete `/v1/messages` endpoint support
- **Multiple Provider Support**: OpenAI, Azure OpenAI, local models (Ollama), and any OpenAI-compatible API
- **Smart Model Mapping**: Configure BIG, MIDDLE, and SMALL models via environment variables
- **Function Calling**: Complete tool use support with proper conversion
- **Streaming Responses**: Real-time SSE streaming support
- **Image Support**: Base64 encoded image input
- **Custom Headers**: Automatic injection of custom HTTP headers for API requests
- **Error Handling**: Comprehensive error handling and logging
- **Auto Port Detection**: Automatically detects and uses available ports

## 📚 Documentation

- **[详细使用指南](./USAGE.md)** - 完整的使用说明，包括各种使用场景、配置详解、测试验证和常见问题
- **[功能对比](./FEATURES.md)** - 与参考实现的功能对比

## Quick Start

### 1. Build

Using Makefile (recommended):
```bash
make build
```

Or manually:
```bash
go build -o claude-with-openai-api
```

### 2. Configure

```bash
cp env.example .env
# Edit .env and add your API configuration
```

### 3. Install (Optional)

Install the binary to your PATH for easy access:

```bash
# Install to ~/.local/bin (recommended, no sudo required)
make install

# Or install to system directory (requires sudo)
make install-system
```

After installation, you can run `claude-with-openai-api` from anywhere.

### 4. Start Server

Using Makefile:
```bash
make run    # Build and run
# or
make start  # Run existing binary
```

Or manually:
```bash
# The server will automatically detect an available port starting from 10086
./claude-with-openai-api
# or if installed:
claude-with-openai-api
```

### 5. Use with Claude Code CLI

```bash
# If ANTHROPIC_API_KEY is not set in the proxy:
ANTHROPIC_BASE_URL=http://localhost:10086 ANTHROPIC_API_KEY="any-value" claude

# If ANTHROPIC_API_KEY is set in the proxy:
ANTHROPIC_BASE_URL=http://localhost:10086 ANTHROPIC_API_KEY="exact-matching-key" claude
```

> 💡 **提示**: 查看 [详细使用指南](./USAGE.md) 了解更多使用场景和配置选项。

## Configuration

The application automatically loads environment variables from a `.env` file in the project root. You can also set environment variables directly in your shell.

### Environment Variables

**Required:**

- `OPENAI_API_KEY` - Your API key for the target provider

**Security:**

- `ANTHROPIC_API_KEY` - Expected Anthropic API key for client validation
  - If set, clients must provide this exact API key to access the proxy
  - If not set, any API key will be accepted

**Model Configuration:**

- `BIG_MODEL` - Model for Claude opus requests (default: `gpt-4o`)
- `MIDDLE_MODEL` - Model for Claude sonnet requests (default: `gpt-4o`)
- `SMALL_MODEL` - Model for Claude haiku requests (default: `gpt-4o-mini`)

**API Configuration:**

- `OPENAI_BASE_URL` - API base URL (default: `https://api.openai.com/v1`)
- `AZURE_API_VERSION` - Azure API version (for Azure OpenAI)

**Server Settings:**

- `HOST` - Server host (default: `0.0.0.0`)
- `PORT` - Server port (default: `10086`, auto-detects if busy)
- `LOG_LEVEL` - Logging level (default: `INFO`)

**Performance:**

- `MAX_TOKENS_LIMIT` - Token limit (default: `4096`)
- `MIN_TOKENS_LIMIT` - Minimum token limit (default: `100`)
- `REQUEST_TIMEOUT` - Request timeout in seconds (default: `90`)

**Custom Headers:**

- `CUSTOM_HEADER_*` - Custom headers for API requests (e.g., `CUSTOM_HEADER_ACCEPT`, `CUSTOM_HEADER_AUTHORIZATION`)

### Model Mapping

The proxy maps Claude model requests to your configured models:

| Claude Request          | Mapped To      | Environment Variable   |
| ----------------------- | -------------- | ---------------------- |
| Models with "haiku"     | `SMALL_MODEL`  | Default: `gpt-4o-mini` |
| Models with "sonnet"    | `MIDDLE_MODEL` | Default: `gpt-4o`      |
| Models with "opus"      | `BIG_MODEL`    | Default: `gpt-4o`      |

## Development

### Using Makefile

```bash
# Install dependencies
make deps

# Build the application
make build

# Run the application (build + run)
make run

# Run existing binary
make start

# Format code
make fmt

# Run tests
make test

# Clean build artifacts
make clean

# Install to PATH
make install

# Uninstall from PATH
make uninstall

# Show help
make help
```

### Manual Commands

```bash
# Install dependencies
go mod tidy

# Run server
go run main.go

# Build
go build -o claude-with-openai-api

# Format code
go fmt ./...
```

## 📖 More Resources

- [详细使用指南](./USAGE.md) - 完整的使用说明和示例
- [功能对比文档](./FEATURES.md) - 功能实现状态
- [参考实现](https://github.com/fuergaosi233/claude-code-proxy) - 原始 Python 实现

## License

MIT License

