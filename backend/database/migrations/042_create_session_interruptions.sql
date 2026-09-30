-- UP Migration
-- 会话中断事件表。用于把"客户端/上游/转换导致的中断"落成可查询的数据，
-- 供中断监控与自动发现扫描（scripts/monitor-interruptions.sh + 后端 InterruptionMonitor）。
--
-- 语义说明：
--   interruption_cause: 中断分类枚举（见 backend/types/interruption.go）
--   dimension: 'subjective'(用户主动停，不上告警不重启) | 'infrastructure'(代理/上游故障，参与告警/自愈)
--   stage:       复用现有阶段枚举 conversion/request/streaming/response
--   detail:      截断≤500 的错误/上下文
--
-- 与 proxy_errors 的区别：
--   proxy_errors 只记上游/协议错误（openai_client.logProxyError 一处写入）。
--   本表额外覆盖 *客户端主动断开/取消* 与 *上游 stall* 这类不产生 HTTP 错误、
--   但会造成会话中断的事件，二者按 request_id / session_id 交叉关联。
CREATE TABLE IF NOT EXISTS session_interruptions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id    TEXT,
  request_id    TEXT,
  config_id     TEXT,
  config_name   TEXT,
  model         TEXT,
  user_id       INTEGER,
  client_ip     TEXT,
  interruption_cause TEXT NOT NULL,
  dimension     TEXT NOT NULL,
  stage         TEXT NOT NULL DEFAULT 'streaming',
  detail        TEXT,
  duration_ms   INTEGER,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_interruptions_cause     ON session_interruptions(interruption_cause);
CREATE INDEX IF NOT EXISTS idx_interruptions_dimension ON session_interruptions(dimension);
CREATE INDEX IF NOT EXISTS idx_interruptions_session   ON session_interruptions(session_id);
CREATE INDEX IF NOT EXISTS idx_interruptions_config    ON session_interruptions(config_id);
CREATE INDEX IF NOT EXISTS idx_interruptions_created   ON session_interruptions(created_at);

-- DOWN Migration
-- DROP TABLE IF EXISTS session_interruptions;