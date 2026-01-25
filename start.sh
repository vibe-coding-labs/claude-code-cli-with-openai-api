#!/bin/bash

# 项目启动脚本 - 后台启动前端和后端，支持热加载
# 特性：
# 1. 自动杀死之前的进程，保证单实例运行
# 2. 后台启动，即使重启 IDE 也不会被重启
# 3. 支持热加载（前端使用 react-scripts，后端使用 air）

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

# PID 文件路径
FRONTEND_PID_FILE="$PROJECT_ROOT/.frontend.pid"
BACKEND_PID_FILE="$PROJECT_ROOT/.backend.pid"

# 日志文件路径
LOG_DIR="$PROJECT_ROOT/logs"
FRONTEND_LOG="$LOG_DIR/frontend.log"
BACKEND_LOG="$LOG_DIR/backend.log"

# 创建日志目录
mkdir -p "$LOG_DIR"

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查进程是否存在
is_process_running() {
    local pid=$1
    if [ -z "$pid" ]; then
        return 1
    fi
    if ps -p "$pid" > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# 杀死进程及其子进程
kill_process_tree() {
    local pid=$1
    local name=$2
    
    if is_process_running "$pid"; then
        print_info "正在停止 $name (PID: $pid)..."
        
        # 获取所有子进程
        local children=$(pgrep -P "$pid" 2>/dev/null || true)
        
        # 先杀死子进程
        if [ -n "$children" ]; then
            for child in $children; do
                kill -TERM "$child" 2>/dev/null || true
            done
        fi
        
        # 杀死主进程
        kill -TERM "$pid" 2>/dev/null || true
        
        # 等待进程结束（最多5秒）
        local count=0
        while is_process_running "$pid" && [ $count -lt 50 ]; do
            sleep 0.1
            count=$((count + 1))
        done
        
        # 如果还没结束，强制杀死
        if is_process_running "$pid"; then
            print_warning "$name 未响应 SIGTERM，使用 SIGKILL 强制终止..."
            kill -9 "$pid" 2>/dev/null || true
            sleep 0.5
        fi
        
        if is_process_running "$pid"; then
            print_error "无法停止 $name (PID: $pid)"
            return 1
        else
            print_success "$name 已停止"
            return 0
        fi
    else
        print_info "$name 未运行"
        return 0
    fi
}

# 停止前端
stop_frontend() {
    if [ -f "$FRONTEND_PID_FILE" ]; then
        local pid=$(cat "$FRONTEND_PID_FILE")
        kill_process_tree "$pid" "前端服务"
        rm -f "$FRONTEND_PID_FILE"
    else
        # 尝试通过端口查找进程
        local pid=$(lsof -ti:54990 2>/dev/null || true)
        if [ -n "$pid" ]; then
            print_warning "发现占用端口 54990 的进程 (PID: $pid)，正在停止..."
            kill_process_tree "$pid" "前端服务"
        fi
    fi
}

# 停止后端
stop_backend() {
    if [ -f "$BACKEND_PID_FILE" ]; then
        local pid=$(cat "$BACKEND_PID_FILE")
        kill_process_tree "$pid" "后端服务"
        rm -f "$BACKEND_PID_FILE"
    else
        # 尝试通过端口查找进程（默认端口 8083）
        local pid=$(lsof -ti:8083 2>/dev/null || true)
        if [ -n "$pid" ]; then
            print_warning "发现占用端口 8083 的进程 (PID: $pid)，正在停止..."
            kill_process_tree "$pid" "后端服务"
        fi
    fi
}

# 启动前端
start_frontend() {
    print_info "正在启动前端服务..."
    
    cd "$PROJECT_ROOT/frontend"
    
    # 检查 node_modules 是否存在
    if [ ! -d "node_modules" ]; then
        print_info "首次运行，正在安装前端依赖..."
        npm install
    fi
    
    # 后台启动前端，输出重定向到日志文件
    nohup npm start > "$FRONTEND_LOG" 2>&1 &
    local pid=$!
    
    # 保存 PID
    echo "$pid" > "$FRONTEND_PID_FILE"
    
    print_success "前端服务已启动 (PID: $pid)"
    print_info "前端日志: $FRONTEND_LOG"
    print_info "前端地址: http://localhost:54990"
}

# 启动后端
start_backend() {
    print_info "正在启动后端服务..."
    
    cd "$PROJECT_ROOT"
    
    # 设置 Go bin 路径
    export GOPATH=$(go env GOPATH)
    export PATH="$GOPATH/bin:$PATH"
    
    # 检查是否安装了 air（用于热加载）
    if ! command -v air &> /dev/null; then
        print_warning "未安装 air，正在安装..."
        go install github.com/air-verse/air@latest
    fi
    
    # 检查是否存在 .air.toml 配置文件
    if [ ! -f ".air.toml" ]; then
        print_info "创建 air 配置文件..."
        cat > .air.toml << 'EOF'
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = ["server"]
  bin = "./tmp/main"
  cmd = "go build -tags dev -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "frontend", "data", "logs"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  post_cmd = []
  pre_cmd = []
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
EOF
    fi
    
    # 后台启动后端，输出重定向到日志文件
    nohup air > "$BACKEND_LOG" 2>&1 &
    local pid=$!
    
    # 保存 PID
    echo "$pid" > "$BACKEND_PID_FILE"
    
    print_success "后端服务已启动 (PID: $pid)"
    print_info "后端日志: $BACKEND_LOG"
    print_info "后端地址: http://localhost:8083"
}

# 显示状态
show_status() {
    echo ""
    print_info "========== 服务状态 =========="
    
    # 前端状态
    if [ -f "$FRONTEND_PID_FILE" ]; then
        local pid=$(cat "$FRONTEND_PID_FILE")
        if is_process_running "$pid"; then
            print_success "前端服务: 运行中 (PID: $pid)"
        else
            print_error "前端服务: 已停止 (PID 文件存在但进程不存在)"
            rm -f "$FRONTEND_PID_FILE"
        fi
    else
        print_warning "前端服务: 未运行"
    fi
    
    # 后端状态
    if [ -f "$BACKEND_PID_FILE" ]; then
        local pid=$(cat "$BACKEND_PID_FILE")
        if is_process_running "$pid"; then
            print_success "后端服务: 运行中 (PID: $pid)"
        else
            print_error "后端服务: 已停止 (PID 文件存在但进程不存在)"
            rm -f "$BACKEND_PID_FILE"
        fi
    else
        print_warning "后端服务: 未运行"
    fi
    
    echo ""
    print_info "查看日志:"
    print_info "  前端: tail -f $FRONTEND_LOG"
    print_info "  后端: tail -f $BACKEND_LOG"
    echo ""
    print_info "停止服务: ./start.sh stop"
    print_info "重启服务: ./start.sh restart"
    print_info "================================"
}

# 主函数
main() {
    local command=${1:-start}
    
    case "$command" in
        start)
            print_info "正在启动服务..."
            stop_frontend
            stop_backend
            start_frontend
            start_backend
            show_status
            ;;
        stop)
            print_info "正在停止服务..."
            stop_frontend
            stop_backend
            print_success "所有服务已停止"
            ;;
        restart)
            print_info "正在重启服务..."
            stop_frontend
            stop_backend
            sleep 1
            start_frontend
            start_backend
            show_status
            ;;
        status)
            show_status
            ;;
        logs)
            local service=${2:-all}
            case "$service" in
                frontend)
                    tail -f "$FRONTEND_LOG"
                    ;;
                backend)
                    tail -f "$BACKEND_LOG"
                    ;;
                all|*)
                    print_info "同时查看前端和后端日志 (Ctrl+C 退出)..."
                    tail -f "$FRONTEND_LOG" "$BACKEND_LOG"
                    ;;
            esac
            ;;
        *)
            echo "用法: $0 {start|stop|restart|status|logs [frontend|backend|all]}"
            echo ""
            echo "命令说明:"
            echo "  start   - 启动前端和后端服务（会先停止已有进程）"
            echo "  stop    - 停止所有服务"
            echo "  restart - 重启所有服务"
            echo "  status  - 查看服务状态"
            echo "  logs    - 查看日志（可选参数: frontend, backend, all）"
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"
