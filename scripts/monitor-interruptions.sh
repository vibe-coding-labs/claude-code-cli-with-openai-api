#!/bin/bash
# scripts/monitor-interruptions.sh — 会话中断监控决策引擎（替换 monitor-client-disconnections.sh）
#
# 读取 session_interruptions 表（cause+dimension 分类，杜绝字符串 grep），按阈值分级：
#   - subjective（用户主动停）量多 → 仅通知/上仪表盘，绝不触发重启
#   - infrastructure（代理/上游故障）≥ Critical 且可复现故障 → 才允许重启（默认 dry-run）
#
# 用法：
#   ./scripts/monitor-interruptions.sh [--db PATH] [--dry-run | --apply]
#
# 默认 --dry-run：只写 decision，不下发重启命令。设置环境变量
#   MONITOR_AUTORESTART=1 才真正执行 systemctl --user restart。
#
# 无参默认等价：--db 自动探测 + --dry-run
#
# 输出约定（供测试断言）：
#   每行一个 deterministic decision，前缀 `DECISION `：
#     DECISION ok
#     DECISION warn reason="..." level=warn
#     DECISION critical reason="..." level=critical
#     DECISION restart reason="..." restart=1            (仅 --apply 时出现)
#     DECISION no-restart reason="..."                    (应重启但被 dry-run/冷却挡住)
#   notify.sh 负责多通道触达并写 scripts/monitor.log

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common/notification.sh"

# ---------- 配置（可被环境变量覆盖） ----------
# 阈值，与 backend/handler/interruption_monitor.go 的默认值保持一致
CRIT_INFRA_5M="${CRIT_INFRA_5M:-25}"          # infrastructure global ≥ X / 5min -> critical
WARN_INFRA_5M="${WARN_INFRA_5M:-8}"
WARN_INFRA_15M_PERCFG="${WARN_INFRA_15M_PERCFG:-5}"   # 单配置 infrastructure ≥ X / 15min
CRIT_INFRA_15M_PERCFG="${CRIT_INFRA_15M_PERCFG:-15}"
WARN_STALL_15M="${WARN_STALL_15M:-10}"        # upstream_stall 全局 ≥ X / 15min
CRIT_STALL_15M="${CRIT_STALL_15M:-30}"
FLAP_1H="${FLAP_1H:-4}"                       # 同一会话 1h 内被打断 ≥ X 次
RESTART_INFRA_5M="${RESTART_INFRA_5M:-20}"    # 重启闸门：infra global ≥ X / 5min
RESTART_MIX_MIN="${RESTART_MIX_MIN:-0.60}"    # 且可复现故障(upstream_stall+rate_limit+conversion_error)占比 ≥ 60%

# 冷却与上限
RESTART_COOLDOWN_S="${RESTART_COOLDOWN_S:-1200}"  # 20min 内不重复重启
RESTART_MAX_PER_HOUR="${RESTART_MAX_PER_HOUR:-3}" # 每小时最多 3 次

STATE_DIR="${MONITOR_STATE_DIR:-$HOME/.local/state/claude-proxy-monitor}"
RESTART_STATE="$STATE_DIR/restart.state"          # "epoch last  count_in_hour"
LOCK_FILE="$STATE_DIR/monitor.lock"

# 服务名（systemd user）
SERVICE_NAME="${MONITOR_SERVICE_NAME:-claude-openai-proxy.service}"

# ---------- 命令行参数 ----------
MODE="dry-run"
DB_PATH=""
while [ $# -gt 0 ]; do
    case "$1" in
        --db) DB_PATH="$2"; shift 2 ;;
        --apply) MODE="apply"; shift ;;
        --dry-run) MODE="dry-run"; shift ;;
        *) echo "未知参数: $1" >&2; exit 64 ;;
    esac
done

if [ -z "${MONITOR_AUTORESTART:-}" ] && [ "$MODE" = "apply" ]; then
    MODE="dry-run"   # 未显式开闸则永不真重启
fi

# ---------- 探测数据库 ----------
if [ -z "$DB_PATH" ]; then
    for cand in \
        "$SCRIPT_DIR/../data/proxy.db" \
        "$HOME/github/vibe-coding-labs/claude-code-cli-with-openai-api/data/proxy.db"; do
        [ -f "$cand" ] && DB_PATH="$cand" && break
    done
fi
if [ -z "$DB_PATH" ] || [ ! -f "$DB_PATH" ]; then
    notify "critical" "session-interruption monitor" "找不到 proxy.db"
    echo "DECISION error reason=\"db-not-found\""
    exit 1
fi

SQLITE="${MONITOR_SQLITE:-$(command -v sqlite3)}"
[ -n "$SQLITE" ] || { echo "DECISION error reason=\"sqlite3-not-found\""; exit 1; }

# RFC3339 UTC cutoff（与 created_at 存储格式一致），epoch 计算保证精确
cutoff_epoch() { echo "$(( $(date +%s) - $1 ))"; }
rc3339() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo; }

now_epoch="$(date +%s)"
c5="$(rc3339 "$(cutoff_epoch 300)")"
c15="$(rc3339 "$(cutoff_epoch 900)")"
c60="$(rc3339 "$(cutoff_epoch 3600)")"

q() { "$SQLITE" "$DB_PATH" "$1" 2>/dev/null; }

# ---------- 1. infrastructure 全局 5min / 15min ----------
infra5="$(q "SELECT COUNT(*) FROM session_interruptions WHERE dimension='infrastructure' AND created_at >= '$c5'")"
infra15="$(q "SELECT COUNT(*) FROM session_interruptions WHERE dimension='infrastructure' AND created_at >= '$c15'")"

# ---------- 2. 单配置 infrastructure 15min ----------
per_cfg_infra="$(q "SELECT COALESCE(NULLIF(config_name,''), config_id, '') || '=' || COUNT(*) FROM session_interruptions WHERE dimension='infrastructure' AND created_at >= '$c15' GROUP BY COALESCE(NULLIF(config_name,''), config_id, '') HAVING COUNT(*) >= $WARN_INFRA_15M_PERCFG ORDER BY COUNT(*) DESC")"

# ---------- 3. 可复现故障 mix（重启闸门用） ----------
stall15="$(q "SELECT COUNT(*) FROM session_interruptions WHERE interruption_cause='upstream_stall' AND created_at >= '$c15'")"
rate_limit15="$(q "SELECT COUNT(*) FROM session_interruptions WHERE interruption_cause='rate_limit' AND created_at >= '$c15'")"
conv15="$(q "SELECT COUNT(*) FROM session_interruptions WHERE interruption_cause='conversion_error' AND created_at >= '$c15'")"
mix_repro="$(q "SELECT COUNT(*) FROM session_interruptions WHERE dimension='infrastructure' AND interruption_cause IN ('upstream_stall','rate_limit','conversion_error') AND created_at >= '$c15'")"

# ---------- 4. flapping 会话 1h ----------
flap="$(q "SELECT COUNT(*) FROM (SELECT session_id FROM session_interruptions WHERE session_id IS NOT NULL AND session_id != '' AND created_at >= '$c60' GROUP BY session_id HAVING COUNT(*) >= $FLAP_1H)")"

# ---------- 5. subjective 量（仅统计，不告警不重启） ----------
subj15="$(q "SELECT COUNT(*) FROM session_interruptions WHERE dimension='subjective' AND created_at >= '$c15'")"

# ---------- 判定 ----------
decision="ok"
reasons=()
level="info"

# 可复现故障占比（乘100避免浮点）
restart_eligible=0
if [ "$infra15" -gt 0 ] && [ "$mix_repro" -gt 0 ]; then
    pct=$(( mix_repro * 100 / infra15 ))
    [ "$pct" -ge "$(awk 'BEGIN{printf "%d", 100*'"$RESTART_MIX_MIN"'}')" ] && restart_eligible=1
fi

notify_level=""
notify_title=""
notify_body=""

if [ "$infra5" -ge "$CRIT_INFRA_5M" ]; then
    decision="critical"; level="critical"
    reasons+=("infra5m=${infra5}>=${CRIT_INFRA_5M}")
elif [ "$infra5" -ge "$WARN_INFRA_5M" ]; then
    [ "$decision" != "critical" ] && decision="warn"; [ "$level" = "info" ] && level="warn"
    reasons+=("infra5m=${infra5}>=${WARN_INFRA_5M}")
fi

if [ -n "$per_cfg_infra" ]; then
    while IFS= read -r line; do
        name="${line%%=*}"; n="${line##*=}"
        if [ "$n" -ge "$CRIT_INFRA_15M_PERCFG" ]; then
            decision="critical"; level="critical"; reasons+=("cfg:${name}=${n}>=${CRIT_INFRA_15M_PERCFG}")
        else
            [ "$decision" != "critical" ] && decision="warn"; [ "$level" = "info" ] && level="warn"
            reasons+=("cfg:${name}=${n}>=${WARN_INFRA_15M_PERCFG}")
        fi
    done <<< "$per_cfg_infra"
fi

if [ "$stall15" -ge "$CRIT_STALL_15M" ]; then
    decision="critical"; level="critical"; reasons+=("stall15=${stall15}>=${CRIT_STALL_15M}")
elif [ "$stall15" -ge "$WARN_STALL_15M" ]; then
    [ "$decision" != "critical" ] && decision="warn"; [ "$level" = "info" ] && level="warn"
    reasons+=("stall15=${stall15}>=${WARN_STALL_15M}")
fi

if [ "${flap:-0}" -gt 0 ]; then
    [ "$decision" != "critical" ] && decision="warn"; [ "$level" = "info" ] && level="warn"
    reasons+=("flapping=${flap}")
fi

# ---------- 重启判定（仅 infrastructure 可复现故障） ----------
should_restart=0
if [ "$infra5" -ge "$RESTART_INFRA_5M" ] && [ "$restart_eligible" -eq 1 ]; then
    should_restart=1
    [ "$level" = "info" ] && level="warn"
fi

# ---------- 触达 ----------
notify_body="infra5m=${infra5} infra15m=${infra15} stall15=${stall15} repro_mix=${mix_repro}/${infra15} subj15=${subj15} flap=${flap:-0} reasons=$(IFS=';'; echo "${reasons[*]}")"
if [ "$decision" != "ok" ]; then
    notify "$level" "session-interruption ${level}: ${decision}" "$notify_body"
fi

# ---------- 聚合判定（每行一个，先打印总等级再打印重启闸门） ----------
agg_reason="$(IFS=';'; echo "${reasons[*]:-no-signals}")"
echo "DECISION $decision reason=\"$agg_reason\""

# ---------- 重启执行（冷却 + 每小时上限，全部受 flock 保护） ----------
if [ "$should_restart" -eq 1 ]; then
    mkdir -p "$STATE_DIR"
    (
        flock -x 9
        last=0; cnt=0
        if [ -f "$RESTART_STATE" ]; then
            read -r last cnt _ < "$RESTART_STATE" 2>/dev/null || { last=0; cnt=0; }
        fi
        elapsed=$(( now_epoch - last ))
        # 重置小时计数
        if [ "$elapsed" -ge 3600 ]; then cnt=0; fi
        in_cooldown=0
        if [ "$elapsed" -lt "$RESTART_COOLDOWN_S" ]; then in_cooldown=1; fi

        if [ "$in_cooldown" -eq 1 ]; then
            echo "DECISION no-restart reason=\"cooldown-${elapsed}s\""
            [ "$level" != "critical" ] && level="critical"
            notify "$level" "session-interruption critical: restart suppressed by cooldown" "$notify_body"
        elif [ "$cnt" -ge "$RESTART_MAX_PER_HOUR" ]; then
            echo "DECISION no-restart reason=\"hourly-limit-${cnt}/${RESTART_MAX_PER_HOUR}\""
            [ "$level" != "critical" ] && level="critical"
            notify "critical" "session-interruption critical: restart hit hourly limit; consider switching config" "$notify_body"
        else
            if [ "$MODE" = "apply" ]; then
                notify "critical" "session-interruption: restarting ${SERVICE_NAME}" "$agg_reason"
                systemctl --user restart "$SERVICE_NAME" >/dev/null 2>&1
                echo "DECISION restart reason=\"$agg_reason\" restart=1"
                echo "$now_epoch $(( cnt + 1 ))" > "$RESTART_STATE"
            else
                notify "critical" "session-interruption: restart NEEDED (dry-run)" "$notify_body"
                echo "DECISION no-restart reason=\"dry-run; infra5m=${infra5}>=${RESTART_INFRA_5M} repro=${mix_repro}/${infra15}\""
            fi
        fi
    ) 9>"$LOCK_FILE"
fi

exit 0