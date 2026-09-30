#!/bin/bash
# scripts/common/notification.sh — 多通道告警触达辅助库
#
# 供 scripts/monitor-interruptions.sh 引用。依次尝试：
#   1. Webhook（Slack/飞书兼容 payload）——若配置了 MONITOR_WEBHOOK_URL
#   2. notify-send 桌面通知
#   3. 始终追加确定性日志行 scripts/monitor.log（供测试/CI 断言）
#
# 用法（作为库 source 后调用）：
#   notify <level> <title> <body>      # level ∈ warn|critical|info
#
# 环境变量（可在 scripts/monitor.env 或 ~/.config/claude-proxy-monitor.env 配置）：
#   MONITOR_WEBHOOK_URL   Slack/飞书 Webhook URL；为空则回落桌面通知/日志

if [ -n "${MONITOR_LOG_PATH:-}" ]; then
    NOTIFY_LOG_FILE="$MONITOR_LOG_PATH"
else
    NOTIFY_LOG_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/monitor.log"
fi

notify() {
    local level="$1"
    local title="$2"
    local body="$3"

    # 始终写确定性日志（每行一个事件，格式稳定供 shell 测试断言）
    printf '[%s] [%s] %s :: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$level" "$title" "$body" >> "$NOTIFY_LOG_FILE"

    # 1) Webhook 优先
    if [ -n "${MONITOR_WEBHOOK_URL:-}" ]; then
        local emoji=":bell:"
        case "$level" in
            critical) emoji=":rotating_light:" ;;
            warn)     emoji=":warning:" ;;
        esac
        # Slack 兼容 payload（飞书同样接受 text 字段）
        curl -sf -X POST -H 'Content-Type: application/json' \
            -d "$(printf '{"text":"%s %s:\\n%s","attachments":[{"color":"%s","blocks":[{"type":"section","text":{"type":"mrkdwn","text":"*%s*\\n%s"}}]}]}' \
                "$emoji" "$title" "$body" "${level}" "$title" "$body")" \
            "$MONITOR_WEBHOOK_URL" >/dev/null 2>&1 && return 0
    fi

    # 2) 桌面通知
    if command -v notify-send >/dev/null 2>&1; then
        local urgency="normal"
        case "$level" in
            critical) urgency="critical" ;;
            warn)     urgency="normal" ;;
        esac
        notify-send --app-name="claude-proxy-monitor" --urgency="$urgency" "$title" "$body" >/dev/null 2>&1 || true
    fi

    return 0
}