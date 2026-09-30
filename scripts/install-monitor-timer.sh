#!/bin/bash
# scripts/install-monitor-timer.sh — 安装并启用会话中断监控 systemd user timer
#
# 将 scripts/systemd/claude-proxy-monitor.{timer,service} 模板拷贝到
# ~/.config/systemd/user/，daemon-reload 并 enable --now（每 2 分钟跑一次）。
#
# 用法：
#   ./scripts/install-monitor-timer.sh          # 安装 + enable
#   ./scripts/install-monitor-timer.sh --uninstall
#
# 默认 service 为 --dry-run（只记 decision 不重启）。观察期后开自动重启闸门：
#   编辑 ~/.config/systemd/user/claude-proxy-monitor.service，给 ExecStart
#   前的 Environment 行加上 MONITOR_AUTORESTART=1，然后 `systemctl --user daemon-reload`。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
UNIT_DIR="$HOME/.config/systemd/user"
NAME="claude-proxy-monitor"

if [ "${1:-}" = "--uninstall" ]; then
    systemctl --user disable --now "$NAME.timer" 2>/dev/null || true
    rm -f "$UNIT_DIR/$NAME.timer" "$UNIT_DIR/$NAME.service"
    systemctl --user daemon-reload
    echo "✅ 已卸载 $NAME.timer"
    exit 0
fi

mkdir -p "$UNIT_DIR"
# 模板里用实际 repo 路径替换 ExecStart（目录位置可能被移动过）
sed "s|/home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/scripts/monitor-interruptions.sh|$REPO_ROOT/scripts/monitor-interruptions.sh|g" \
    "$SCRIPT_DIR/systemd/$NAME.service" > "$UNIT_DIR/$NAME.service"
cp "$SCRIPT_DIR/systemd/$NAME.timer" "$UNIT_DIR/$NAME.timer"

systemctl --user daemon-reload
systemctl --user enable --now "$NAME.timer"

echo "✅ 已安装并启用 systemd user timer: $NAME.timer"
echo "   每 2 分钟跑一次（--dry-run，只记录不自动重启）。"
echo ""
echo "   查看下次触发时间: systemctl --user list-timers $NAME.timer"
echo "   查看最近一次运行:  systemctl --user status $NAME.timer"
echo "   决策日志:          $REPO_ROOT/scripts/monitor.log"
echo ""
echo "   观察 1-2 周后如需开启自动重启，编辑 $UNIT_DIR/$NAME.service"
echo "   并加入 Environment=MONITOR_AUTORESTART=1，然后 "
echo "   systemctl --user daemon-reload"