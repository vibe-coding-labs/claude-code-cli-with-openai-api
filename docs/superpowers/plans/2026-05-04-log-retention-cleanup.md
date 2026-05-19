# Log Retention & Auto-Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修复日志详情页404 bug，添加系统设置表和日志保留策略，实现定时自动清理过期请求日志防止磁盘爆满。

**Architecture:** 用户在系统设置页面配置日志保留天数 → 保存到 system_settings 表 → 后端定时任务（每日2AM）读取保留设置 → 调用 DeleteOldRequestLogs 清理过期记录。复用现有 monitor.go 的 CleanupOldData 机制。

**Tech Stack:** Go 1.22, Gin, SQLite, React 18, Ant Design 5

**Risks:**
- Bug修复涉及 ui.go 和 server.go 两个入口文件需同步 — 缓解：server.go 已有该路由，只需修 ui.go
- 定时任务删除大量数据时可能锁表 — 缓解：使用 LIMIT 分批删除

---

### Task 1: Fix Log Detail 404 Bug

**Depends on:** None
**Files:**
- Modify: `backend/cmd/ui.go:173-174`（在 logs 路由组中添加 detail 路由）

- [ ] **Step 1: 在 ui.go 中添加日志详情路由 — 补齐 server.go 已有但 ui.go 缺失的路由**
文件: `backend/cmd/ui.go:173-174`

```go
// 替换 backend/cmd/ui.go:173-174 两行为以下三行
			configAPI.GET("/configs/:id/logs", h.GetConfigLogs)
			configAPI.GET("/configs/:id/logs/:log_id", h.GetLogDetail)
			configAPI.DELETE("/configs/:id/logs", h.DeleteConfigLogs)
```

- [ ] **Step 2: 验证日志详情API返回401而非404**
Run: `curl -s -o /dev/null -w "%{http_code}" http://localhost:54988/api/configs/5b683cf6-d28e-41eb-a110-201a8ca0c816/logs/1`
Expected:
  - Output contains: "401"
  - Output does NOT contain: "404"

---

### Task 2: System Settings Data Layer

**Depends on:** None
**Files:**
- Create: `backend/database/migrations/036_create_system_settings_table.sql`
- Create: `backend/database/settings.go`
- Modify: `backend/database/logs.go:317`（添加 DeleteOldRequestLogs 函数）

- [ ] **Step 1: 创建 system_settings 表迁移 — 存储系统级配置如日志保留天数**

```sql
-- backend/database/migrations/036_create_system_settings_table.sql
CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT
);

INSERT OR IGNORE INTO system_settings (key, value) VALUES ('log_retention_days', '30');
```

- [ ] **Step 2: 创建 settings.go — 系统设置的 CRUD 函数**

```go
// backend/database/settings.go
package database

import (
	"database/sql"
	"fmt"
	"time"
)

// GetSetting retrieves a system setting by key
func GetSetting(key string) (string, error) {
	var value string
	err := DB.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting: %w", err)
	}
	return value, nil
}

// SetSetting updates or creates a system setting
func SetSetting(key, value, updatedBy string) error {
	_, err := DB.Exec(`
		INSERT INTO system_settings (key, value, updated_at, updated_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by
	`, key, value, time.Now().Format("2006-01-02 15:04:05"), updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set setting: %w", err)
	}
	return nil
}

// GetAllSettings retrieves all system settings
func GetAllSettings() (map[string]string, error) {
	rows, err := DB.Query("SELECT key, value FROM system_settings")
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings[key] = value
	}
	return settings, nil
}
```

- [ ] **Step 3: 在 logs.go 中添加 DeleteOldRequestLogs — 按天数清理过期请求日志**
文件: `backend/database/logs.go:317`（在 DeleteConfigLogs 函数之后添加）

```go
// DeleteOldRequestLogs deletes request logs older than the specified number of days
func DeleteOldRequestLogs(days int) (int64, error) {
	query := `
		DELETE FROM request_logs
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`
	result, err := DB.Exec(query, days)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old request logs: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// GetRequestLogCount returns total number of request logs
func GetRequestLogCount() (int64, error) {
	var count int64
	err := DB.QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count request logs: %w", err)
	}
	return count, nil
}
```

- [ ] **Step 4: 验证迁移和函数**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && DB_PATH=../data/proxy.db go run -tags dev . ui &
sleep 3 && curl -s http://localhost:54988/health && kill %1 2>/dev/null`
Expected:
  - Exit code: 0
  - Output contains: "healthy"
  - Output contains: "036" (migration applied)

---

### Task 3: Backend Settings API + Auto-Cleanup Integration

**Depends on:** Task 2
**Files:**
- Modify: `backend/handler/config_api.go:160`（添加 settings handler）
- Modify: `backend/handler/monitor.go:274`（在 CleanupOldData 中添加 request_logs 清理）
- Modify: `backend/cmd/ui.go:174-180`（添加 settings API 路由）
- Modify: `backend/cmd/server.go:272-280`（添加 settings API 路由）

- [ ] **Step 1: 在 config_api.go 添加 settings API handlers — 读取和更新系统设置**
文件: `backend/handler/config_api.go:160`（在 GetLogDetail 函数之后添加）

```go
// GetSystemSettings retrieves all system settings
func (h *Handler) GetSystemSettings(c *gin.Context) {
	settings, err := database.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// UpdateSystemSettings updates system settings
func (h *Handler) UpdateSystemSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	username := c.GetString("username")
	for key, value := range req {
		if err := database.SetSetting(key, value, username); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save setting %s: %v", key, err)})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// GetLogStats returns log storage statistics
func (h *Handler) GetLogStats(c *gin.Context) {
	totalLogs, err := database.GetRequestLogCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log stats"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total_logs": totalLogs})
}
```

- [ ] **Step 2: 在 ui.go 添加 settings API 路由**
文件: `backend/cmd/ui.go:180`（在 configAPI 路由组的闭合括号之前添加）

```go
// 在 configAPI.DELETE("/proxy-errors", h.CleanupProxyErrors) 之后添加
				configAPI.GET("/settings", h.GetSystemSettings)
				configAPI.PUT("/settings", h.UpdateSystemSettings)
				configAPI.GET("/log-stats", h.GetLogStats)
```

- [ ] **Step 3: 在 server.go 添加相同的 settings API 路由**
文件: `backend/cmd/server.go:280`（在 configAPI 路由组的闭合括号之前，找到对应位置添加）

```go
// 在 configAPI.DELETE("/proxy-errors", ...) 之后添加
				configAPI.GET("/settings", h.GetSystemSettings)
				configAPI.PUT("/settings", h.UpdateSystemSettings)
				configAPI.GET("/log-stats", h.GetLogStats)
```

- [ ] **Step 4: 修改 monitor.go CleanupOldData — 添加 request_logs 自动清理**
文件: `backend/handler/monitor.go:3-9`（添加 `strconv` 到 import）和 `:274-291`（替换整个 CleanupOldData 函数）

import 修改 — 在 `backend/handler/monitor.go:3-12` 的 import 块添加 `"strconv"`:

```go
import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)
```

```go
// CleanupOldData cleans up old logs and stats based on retention settings
func CleanupOldData() error {
	// Get log retention setting, default 30 days
	retentionDays := 30
	if daysStr, err := database.GetSetting("log_retention_days"); err == nil {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			retentionDays = days
		}
	}

	// Delete old request logs based on retention policy
	deleted, err := database.DeleteOldRequestLogs(retentionDays)
	if err != nil {
		fmt.Printf("Warning: failed to delete old request logs: %v\n", err)
	} else if deleted > 0 {
		fmt.Printf("Auto-cleaned %d request logs older than %d days\n", deleted, retentionDays)
	}

	// Delete load balancer request logs older than 30 days
	if err := database.DeleteOldLoadBalancerRequestLogs(30); err != nil {
		return fmt.Errorf("failed to delete old request logs: %w", err)
	}

	// Delete stats older than 90 days
	if err := database.DeleteOldStats(90); err != nil {
		return fmt.Errorf("failed to delete old stats: %w", err)
	}

	// Delete alerts older than 90 days
	if err := database.DeleteOldAlerts(90); err != nil {
		return fmt.Errorf("failed to delete old alerts: %w", err)
	}

	return nil
}
```

- [ ] **Step 5: 验证后端编译和API可访问**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && DB_PATH=../data/proxy.db go build -tags dev -o /dev/null . 2>&1`
Expected:
  - Exit code: 0
  - Output is empty (no compilation errors)

---

### Task 4: Frontend System Settings Page

**Depends on:** Task 3
**Files:**
- Create: `frontend/src/components/SystemSettings.tsx`
- Modify: `frontend/src/App.tsx:40-70`（添加菜单项和路由）

- [ ] **Step 1: 创建 SystemSettings.tsx — 日志保留设置页面**

```tsx
// frontend/src/components/SystemSettings.tsx
import React, { useState, useEffect } from 'react';
import { Card, Form, InputNumber, Button, message, Statistic, Row, Col, Space } from 'antd';
import { SettingOutlined, DeleteOutlined, DatabaseOutlined } from '@ant-design/icons';
import { apiClient } from '../utils/api';

interface SystemSettingsData {
  log_retention_days?: string;
}

const SystemSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [totalLogs, setTotalLogs] = useState<number>(0);

  useEffect(() => {
    loadSettings();
    loadLogStats();
  }, []);

  const loadSettings = async () => {
    setLoading(true);
    try {
      const resp = await apiClient.get('/api/settings');
      const data: SystemSettingsData = resp.data;
      form.setFieldsValue({
        log_retention_days: parseInt(data.log_retention_days || '30', 10),
      });
    } catch {
      message.error('Failed to load settings');
    } finally {
      setLoading(false);
    }
  };

  const loadLogStats = async () => {
    try {
      const resp = await apiClient.get('/api/log-stats');
      setTotalLogs(resp.data.total_logs || 0);
    } catch {
      // ignore
    }
  };

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await apiClient.put('/api/settings', {
        log_retention_days: String(values.log_retention_days),
      });
      message.success('Settings saved successfully');
      loadLogStats();
    } catch {
      message.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}><SettingOutlined /> System Settings</h2>
      <Row gutter={24}>
        <Col span={16}>
          <Card title="Log Retention" loading={loading}>
            <Form form={form} layout="vertical">
              <Form.Item
                name="log_retention_days"
                label="Request Log Retention (days)"
                rules={[
                  { required: true, message: 'Please enter retention days' },
                  { type: 'number', min: 1, max: 365, message: 'Must be between 1-365 days' },
                ]}
                extra="Logs older than this will be automatically deleted daily at 2:00 AM"
              >
                <InputNumber min={1} max={365} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item>
                <Space>
                  <Button type="primary" onClick={handleSave} loading={saving}>
                    Save Settings
                  </Button>
                </Space>
              </Form.Item>
            </Form>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="Log Storage">
            <Statistic
              title="Total Request Logs"
              value={totalLogs}
              prefix={<DatabaseOutlined />}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default SystemSettings;
```

- [ ] **Step 2: 在 App.tsx 添加系统设置菜单项和路由**
文件: `frontend/src/App.tsx`

菜单项修改 — 在 `frontend/src/App.tsx:64-67`（audit-logs 菜单项之后添加）:

```tsx
          {
            key: '/ui/settings',
            icon: <SettingOutlined />,
            label: <Link to="/ui/settings">系统设置</Link>,
          },
```

路由修改 — 在 `frontend/src/App.tsx:148`（audit-logs 路由之后添加）:

```tsx
              <Route path="settings" element={<ProtectedRoute><SystemSettings /></ProtectedRoute>} />
```

import 添加 — 在 `frontend/src/App.tsx` 的 import 区域添加:

```tsx
import SystemSettings from './components/SystemSettings';
```

- [ ] **Step 3: 验证前端编译**
Run: `cd /Users/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/frontend && npx react-scripts build 2>&1 | tail -5`
Expected:
  - Exit code: 0
  - Output contains: "Compiled successfully"
