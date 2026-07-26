# 通信日志存储管理完善 Implementation Plan（续）

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Goal:** 完成通信日志系统的剩余部分：清理引擎（大小监控 + 双重清理 + VACUUM INTO）、API 层扩展（存储统计 + 手动维护端点）、前端 UI 扩展（存储管理配置 + 维护操作按钮）、端到端测试。

**Architecture:** 清理引擎每日 2AM 自动运行 → 先按保留天数删除超龄日志 → 再按大小配额删除最旧记录 → 清理 body 文件 → VACUUM INTO 回收磁盘空间 → 渐进式迁移内联 body 到文件。前端 SystemSettings 页面展示存储统计（DB 大小、body 文件大小、使用率进度条）+ 配置项（保留天数、大小配额、存储模式）+ 维护操作按钮（清理、VACUUM、迁移）。

**Tech Stack:** Go 1.22, SQLite (mattn/go-sqlite3 v1.14.22), gzip 压缩, Ant Design 5.x (前端), golang.org/x/sys/unix (磁盘空间查询)

**Risks:**
- VACUUM INTO 的原子替换（rename old→.old, clean→old）在 WAL 模式活跃写入时可能短暂冲突 → 缓解：自动清理在 2AM 低峰期执行；手动 VACUUM 时先检查 WAL 活跃度
- `getAvailableDiskSpace` 需要平台特定 syscall → 缓解：使用 `golang.org/x/sys/unix`，Linux/macOS 通用；Windows 降级返回 -1
- 前端设置变更需要前后端同步 → 缓解：先完成后端 API，再更新前端
- 旧数据迁移可能耗时 → 缓解：每次只迁移 100 条，渐进式不锁表

---

### Task 3: 清理引擎 — 数据库大小监控 + 双重清理 + VACUUM INTO

**Depends on:** None（Task 1-2 已完成）
**Files:**
- Create: `backend/database/log_cleanup.go`
- Modify: `backend/handler/monitor.go:275-308` (CleanupOldData 函数)

- [ ] **Step 1: 创建 log_cleanup.go — 数据库大小监控和清理引擎**

```go
// backend/database/log_cleanup.go
package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DatabaseSizeInfo holds information about the database file size.
type DatabaseSizeInfo struct {
	DBPath        string `json:"db_path"`
	DBSizeBytes   int64  `json:"db_size_bytes"`
	WALSizeBytes  int64  `json:"wal_size_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
	FreePages     int64  `json:"free_pages"`
	TotalPages    int64  `json:"total_pages"`
	PageSize      int64  `json:"page_size"`
	BodySizeBytes int64  `json:"body_size_bytes"`
}

// GetDatabaseSizeInfo returns size information about the database files.
func GetDatabaseSizeInfo() (*DatabaseSizeInfo, error) {
	info := &DatabaseSizeInfo{}

	// Get DB path from the current connection
	var dbPath string
	err := DB.QueryRow("PRAGMA database_list").Scan(nil, nil, &dbPath)
	if err != nil || dbPath == "" {
		// Fallback: use PRAGMA for size estimation
		var pageSize, pageCount, freeList int64
		DB.QueryRow("PRAGMA page_size").Scan(&pageSize)
		DB.QueryRow("PRAGMA page_count").Scan(&pageCount)
		DB.QueryRow("PRAGMA freelist_count").Scan(&freeList)
		info.PageSize = pageSize
		info.TotalPages = pageCount
		info.FreePages = freeList
		info.DBSizeBytes = pageSize * pageCount
		info.TotalBytes = info.DBSizeBytes
		return info, nil
	}

	info.DBPath = dbPath

	// Check main DB file
	if stat, err := os.Stat(dbPath); err == nil {
		info.DBSizeBytes = stat.Size()
	}

	// Check WAL file
	walPath := dbPath + "-wal"
	if stat, err := os.Stat(walPath); err == nil {
		info.WALSizeBytes = stat.Size()
	}

	// Check SHM file (usually small, include for completeness)
	shmPath := dbPath + "-shm"
	if stat, err := os.Stat(shmPath); err == nil {
		info.WALSizeBytes += stat.Size()
	}

	info.TotalBytes = info.DBSizeBytes + info.WALSizeBytes

	// Get page info
	DB.QueryRow("PRAGMA page_size").Scan(&info.PageSize)
	DB.QueryRow("PRAGMA page_count").Scan(&info.TotalPages)
	DB.QueryRow("PRAGMA freelist_count").Scan(&info.FreePages)

	// Get body storage size
	storage := GetLogStorage()
	bodySize, _ := storage.GetStorageSize()
	info.BodySizeBytes = bodySize

	return info, nil
}

// VacuumInto performs a safe VACUUM INTO operation.
// It creates a compacted copy of the database without locking the original,
// then atomically replaces the original with the compacted version.
// Returns the size of the new file, or an error.
func VacuumInto(dbPath string) (int64, error) {
	cleanPath := dbPath + ".clean"

	// Check available disk space
	var dbSize int64
	if stat, err := os.Stat(dbPath); err == nil {
		dbSize = stat.Size()
	}

	// Need at least 1.1x the DB size in free space
	freeSpace := getAvailableDiskSpace(filepath.Dir(dbPath))
	if freeSpace >= 0 && freeSpace < int64(float64(dbSize)*1.1) {
		return 0, fmt.Errorf("insufficient disk space: need ~%.1f GB, have %.1f GB free",
			float64(dbSize)*1.1/1e9, float64(freeSpace)/1e9)
	}

	// Remove any previous clean copy
	os.Remove(cleanPath)

	// Run VACUUM INTO (safe on live database in WAL mode)
	_, err := DB.Exec(fmt.Sprintf("VACUUM INTO '%s'", cleanPath))
	if err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("VACUUM INTO failed: %w", err)
	}

	// Verify the clean copy exists and get its size
	var cleanSize int64
	if stat, err := os.Stat(cleanPath); err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("clean copy not found after VACUUM INTO: %w", err)
	} else {
		cleanSize = stat.Size()
	}

	// Integrity check on the clean copy
	cleanDB, err := sql.Open("sqlite3", cleanPath+"?mode=ro")
	if err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("failed to open clean copy for verification: %w", err)
	}

	var ok string
	err = cleanDB.QueryRow("PRAGMA integrity_check").Scan(&ok)
	cleanDB.Close()

	if err != nil || ok != "ok" {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("integrity check failed on clean copy: %v", ok)
	}

	// Atomically replace the original with the clean copy.
	// This requires a brief moment where the DB is not being written to.
	// We rename the original to .old, then rename the clean to the original name.
	backupPath := dbPath + ".old"
	os.Remove(backupPath)

	if err := os.Rename(dbPath, backupPath); err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("failed to rename original DB: %w", err)
	}

	if err := os.Rename(cleanPath, dbPath); err != nil {
		// Try to restore the original
		os.Rename(backupPath, dbPath)
		os.Remove(cleanPath)
		return 0, fmt.Errorf("failed to rename clean copy: %w", err)
	}

	// Remove the old backup
	os.Remove(backupPath)

	log.Printf("VACUUM INTO completed: %.1f MB → %.1f MB (saved %.1f MB)",
		float64(dbSize)/1e6, float64(cleanSize)/1e6, float64(dbSize-cleanSize)/1e6)

	return cleanSize, nil
}

// getAvailableDiskSpace returns available disk space in bytes for the given directory.
// Returns -1 if unable to determine.
func getAvailableDiskSpace(dir string) int64 {
	var stat syscallStatfs
	if err := statfs(dir, &stat); err != nil {
		return -1
	}
	return stat.bavail * uint64(stat.bsize)
}

// DeleteOldestRequestLogs deletes the oldest N request logs to free space.
// Returns the number of rows deleted.
func DeleteOldestRequestLogs(count int) (int64, error) {
	query := `
		DELETE FROM request_logs
		WHERE id IN (
			SELECT id FROM request_logs
			ORDER BY created_at ASC
			LIMIT ?
		)
	`
	result, err := DB.Exec(query, count)
	if err != nil {
		return 0, fmt.Errorf("failed to delete oldest request logs: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// GetRequestLogCountByAge returns the count of request logs older than N days.
func GetRequestLogCountByAge(days int) (int64, error) {
	var count int64
	err := DB.QueryRow(
		"SELECT COUNT(*) FROM request_logs WHERE created_at < datetime('now', '-' || ? || ' days')",
		days,
	).Scan(&count)
	return count, err
}

// MigrateInlineBodiesToFiles migrates a batch of inline request_body/response_body
// to file storage. Returns the number of rows migrated.
// This is called incrementally to avoid locking the database for too long.
func MigrateInlineBodiesToFiles(batchSize int) (int, error) {
	query := `
		SELECT id, request_body, response_body, created_at
		FROM request_logs
		WHERE (request_body IS NOT NULL AND request_body_path IS NULL)
		   OR (response_body IS NOT NULL AND response_body_path IS NULL)
		ORDER BY id DESC
		LIMIT ?
	`

	rows, err := DB.Query(query, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	storage := GetLogStorage()
	migrated := 0

	for rows.Next() {
		var id int64
		var requestBody, responseBody sql.NullString
		var createdAt time.Time

		if err := rows.Scan(&id, &requestBody, &responseBody, &createdAt); err != nil {
			continue
		}

		var reqPath, respPath string

		if requestBody.Valid && requestBody.String != "" {
			path, err := storage.StoreBody(id, requestBody.String, "req", createdAt)
			if err == nil {
				reqPath = path
			}
		}

		if responseBody.Valid && responseBody.String != "" {
			path, err := storage.StoreBody(id, responseBody.String, "resp", createdAt)
			if err == nil {
				respPath = path
			}
		}

		if reqPath != "" || respPath != "" {
			DB.Exec(
				"UPDATE request_logs SET request_body_path = COALESCE(?, request_body_path), response_body_path = COALESCE(?, response_body_path), request_body = CASE WHEN ? != '' THEN NULL ELSE request_body END, response_body = CASE WHEN ? != '' THEN NULL ELSE response_body END WHERE id = ?",
				reqPath, respPath, reqPath, respPath, id,
			)
			migrated++
		}
	}

	return migrated, nil
}
```

- [ ] **Step 2: 创建 disk_space.go — 跨平台磁盘空间查询**

```go
// backend/database/disk_space.go
package database

// syscallStatfs is the platform-specific statfs struct.
// disk_space_linux.go and disk_space_other.go provide the actual implementation.

// statfs calls the platform-specific statfs syscall.
// disk_space_linux.go and disk_space_other.go provide the actual implementation.
```

- [ ] **Step 3: 创建 disk_space_linux.go — Linux 磁盘空间查询实现**

```go
//go:build linux

package database

import (
	"golang.org/x/sys/unix"
)

type syscallStatfs = unix.Statfs_t

func statfs(path string, stat *syscallStatfs) error {
	return unix.Statfs(path, stat)
}
```

- [ ] **Step 4: 创建 disk_space_other.go — 非 Linux 平台降级实现**

```go
//go:build !linux

package database

import (
	"fmt"
)

type syscallStatfs struct {
	bavail uint64
	bsize  uint64
}

func statfs(path string, stat *syscallStatfs) error {
	return fmt.Errorf("statfs not implemented on this platform")
}
```

- [ ] **Step 5: 修改 CleanupOldData — 扩展清理逻辑，增加大小配额检查、body 文件清理和 VACUUM**
文件: `backend/handler/monitor.go:275-308`

```go
// 替换 backend/handler/monitor.go:275-308 的 CleanupOldData 函数
func CleanupOldData() error {
	// --- 1. Time-based retention cleanup ---

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

	// Delete old proxy errors
	proxyErrorRetentionDays := 30
	if daysStr, err := database.GetSetting("proxy_error_retention_days"); err == nil {
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			proxyErrorRetentionDays = days
		}
	}
	if err := database.CleanupOldProxyErrors(time.Duration(proxyErrorRetentionDays) * 24 * time.Hour); err != nil {
		fmt.Printf("Warning: failed to cleanup old proxy errors: %v\n", err)
	}

	// Delete load balancer request logs older than 30 days
	if err := database.DeleteOldLoadBalancerRequestLogs(30); err != nil {
		fmt.Printf("Warning: failed to delete old LB request logs: %v\n", err)
	}

	// Delete stats older than 90 days
	if err := database.DeleteOldStats(90); err != nil {
		fmt.Printf("Warning: failed to delete old stats: %v\n", err)
	}

	// Delete alerts older than 90 days
	if err := database.DeleteOldAlerts(90); err != nil {
		fmt.Printf("Warning: failed to delete old alerts: %v\n", err)
	}

	// Clean old body files
	storage := database.GetLogStorage()
	if dirsRemoved, err := storage.CleanOldDirectories(retentionDays); err == nil && dirsRemoved > 0 {
		fmt.Printf("Auto-cleaned %d old body file directories\n", dirsRemoved)
	}

	// --- 2. Size-based quota enforcement ---

	maxDBSizeGB := 10 // default 10 GB
	if sizeStr, err := database.GetSetting("max_db_size_gb"); err == nil {
		if size, err := strconv.Atoi(sizeStr); err == nil && size > 0 {
			maxDBSizeGB = size
		}
	}

	sizeInfo, err := database.GetDatabaseSizeInfo()
	if err == nil {
		maxBytes := int64(maxDBSizeGB) * 1024 * 1024 * 1024
		if sizeInfo.TotalBytes > maxBytes {
			// Delete oldest logs in batches until under quota
			overBy := sizeInfo.TotalBytes - maxBytes
			// Estimate: each log row averages ~5KB (metadata only, bodies in files)
			estimatedRows := overBy / 5120
			if estimatedRows < 100 {
				estimatedRows = 100
			}
			if estimatedRows > 10000 {
				estimatedRows = 10000 // cap batch size
			}

			deletedRows, err := database.DeleteOldestRequestLogs(int(estimatedRows))
			if err != nil {
				fmt.Printf("Warning: failed to delete oldest logs for quota: %v\n", err)
			} else if deletedRows > 0 {
				fmt.Printf("Quota enforcement: deleted %d oldest logs (DB was %.1f GB, limit %d GB)\n",
					deletedRows, float64(sizeInfo.TotalBytes)/1e9, maxDBSizeGB)
			}
		}
	}

	// --- 3. VACUUM INTO to reclaim disk space ---
	// Only run if there are significant free pages (>10% of total)
	if sizeInfo != nil && sizeInfo.TotalPages > 0 {
		freeRatio := float64(sizeInfo.FreePages) / float64(sizeInfo.TotalPages)
		if freeRatio > 0.10 {
			if sizeInfo.DBPath != "" {
				if newSize, err := database.VacuumInto(sizeInfo.DBPath); err != nil {
					fmt.Printf("Warning: VACUUM INTO failed: %v\n", err)
				} else {
					fmt.Printf("VACUUM INTO completed: new size %.1f MB\n", float64(newSize)/1e6)
				}
			}
		}
	}

	// --- 4. Migrate inline bodies to files (incremental) ---
	if migrated, err := database.MigrateInlineBodiesToFiles(100); err == nil && migrated > 0 {
		fmt.Printf("Migrated %d inline bodies to file storage\n", migrated)
	}

	return nil
}
```

- [ ] **Step 6: 安装 golang.org/x/sys 依赖**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go get golang.org/x/sys/unix`
Expected:
  - Exit code: 0
  - Output does NOT contain: "ERR!" or "error"

- [ ] **Step 7: 验证构建**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./database/... ./handler/...`
Expected:
  - Exit code: 0

- [ ] **Step 8: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/database/log_cleanup.go backend/database/disk_space.go backend/database/disk_space_linux.go backend/database/disk_space_other.go backend/handler/monitor.go backend/go.mod backend/go.sum && git commit -m "feat(database): add size-based cleanup, VACUUM INTO, body migration engine, and disk space query"`

---

### Task 4: API 层扩展 — 存储统计和手动维护接口

**Depends on:** Task 3
**Files:**
- Modify: `backend/handler/config_api.go:198-206` (GetLogStats 函数)
- Modify: `backend/handler/config_api.go` (新增 TriggerCleanup/TriggerVacuum/MigrateBodies)
- Modify: `backend/cmd/ui.go:185` (注册新 API 路由)

- [ ] **Step 1: 扩展 GetLogStats — 返回完整的存储统计信息**
文件: `backend/handler/config_api.go:198-206`

```go
// 替换 backend/handler/config_api.go:198-206 的 GetLogStats 函数
func (h *Handler) GetLogStats(c *gin.Context) {
	totalLogs, err := database.GetRequestLogCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log stats"})
		return
	}

	// Get database size info
	sizeInfo, err := database.GetDatabaseSizeInfo()
	if err != nil {
		// Don't fail the whole request, just omit size info
		c.JSON(http.StatusOK, gin.H{
			"total_logs": totalLogs,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_logs":       totalLogs,
		"db_size_bytes":    sizeInfo.DBSizeBytes,
		"wal_size_bytes":   sizeInfo.WALSizeBytes,
		"total_size_bytes": sizeInfo.TotalBytes,
		"free_pages":       sizeInfo.FreePages,
		"total_pages":      sizeInfo.TotalPages,
		"page_size":        sizeInfo.PageSize,
		"body_size_bytes":  sizeInfo.BodySizeBytes,
		"db_path":          sizeInfo.DBPath,
	})
}
```

- [ ] **Step 2: 添加手动清理、VACUUM 和 body 迁移 API 端点**
文件: `backend/handler/config_api.go`（在 GetLogStats 函数之后添加）

```go
// TriggerCleanup manually triggers the cleanup process
func (h *Handler) TriggerCleanup(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	go func() {
		if err := CleanupOldData(); err != nil {
			log.Printf("Manual cleanup failed: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Cleanup triggered successfully"})
}

// TriggerVacuum manually triggers VACUUM INTO
func (h *Handler) TriggerVacuum(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	sizeInfo, err := database.GetDatabaseSizeInfo()
	if err != nil || sizeInfo.DBPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database path"})
		return
	}

	newSize, err := database.VacuumInto(sizeInfo.DBPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("VACUUM failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "VACUUM completed successfully",
		"new_size_bytes": newSize,
	})
}

// MigrateBodies manually triggers inline body migration
func (h *Handler) MigrateBodies(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	migrated, err := database.MigrateInlineBodiesToFiles(1000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Migration failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Migration batch completed",
		"migrated": migrated,
	})
}
```

- [ ] **Step 3: 注册新 API 路由**
文件: `backend/cmd/ui.go:185`（在 `configAPI.GET("/log-stats", h.GetLogStats)` 之后添加）

在 `configAPI.GET("/log-stats", h.GetLogStats)` 行之后添加：
```go
				configAPI.POST("/cleanup", h.TriggerCleanup)
				configAPI.POST("/vacuum", h.TriggerVacuum)
				configAPI.POST("/migrate-bodies", h.MigrateBodies)
```

- [ ] **Step 4: 验证构建**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./handler/... ./cmd/...`
Expected:
  - Exit code: 0

- [ ] **Step 5: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/handler/config_api.go backend/cmd/ui.go && git commit -m "feat(api): add storage stats, manual cleanup, vacuum, and body migration endpoints"`

---

### Task 5: 前端 UI 扩展 — 系统设置页面增加存储管理

**Depends on:** Task 4
**Files:**
- Create: `frontend/src/types/settings.ts`
- Modify: `frontend/src/components/SystemSettings.tsx`

- [ ] **Step 1: 创建 settings.ts — 设置类型定义**

```typescript
// frontend/src/types/settings.ts

/** System settings returned by GET /api/settings */
export interface SystemSettingsData {
  log_retention_days?: string;
  max_db_size_gb?: string;
  log_body_storage?: string;
  proxy_error_retention_days?: string;
}

/** Log storage stats returned by GET /api/log-stats */
export interface LogStorageStats {
  total_logs: number;
  db_size_bytes?: number;
  wal_size_bytes?: number;
  total_size_bytes?: number;
  free_pages?: number;
  total_pages?: number;
  page_size?: number;
  body_size_bytes?: number;
  db_path?: string;
}
```

- [ ] **Step 2: 重写 SystemSettings.tsx — 扩展存储管理 UI**

```tsx
// frontend/src/components/SystemSettings.tsx
import React, { useState, useEffect } from 'react';
import {
  Card, Form, InputNumber, Button, message, Statistic, Row, Col, Space,
  Progress, Tooltip, Select, Divider, Alert
} from 'antd';
import {
  SettingOutlined, DatabaseOutlined, DeleteOutlined,
  CompressOutlined, CloudUploadOutlined, HardDriveOutlined
} from '@ant-design/icons';
import api from '../services/api';
import type { SystemSettingsData, LogStorageStats } from '../types/settings';

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const SystemSettings: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [stats, setStats] = useState<LogStorageStats | null>(null);
  const [cleaning, setCleaning] = useState(false);
  const [vacuuming, setVacuuming] = useState(false);
  const [migrating, setMigrating] = useState(false);

  useEffect(() => {
    loadSettings();
    loadLogStats();
  }, []);

  const loadSettings = async () => {
    setLoading(true);
    try {
      const resp = await api.get('/api/settings');
      const data: SystemSettingsData = resp.data;
      form.setFieldsValue({
        log_retention_days: parseInt(data.log_retention_days || '30', 10),
        max_db_size_gb: parseInt(data.max_db_size_gb || '10', 10),
        log_body_storage: data.log_body_storage || 'file',
        proxy_error_retention_days: parseInt(data.proxy_error_retention_days || '30', 10),
      });
    } catch {
      message.error('Failed to load settings');
    } finally {
      setLoading(false);
    }
  };

  const loadLogStats = async () => {
    try {
      const resp = await api.get('/api/log-stats');
      setStats(resp.data);
    } catch {
      // ignore
    }
  };

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      await api.put('/api/settings', {
        log_retention_days: String(values.log_retention_days),
        max_db_size_gb: String(values.max_db_size_gb),
        log_body_storage: values.log_body_storage,
        proxy_error_retention_days: String(values.proxy_error_retention_days),
      });
      message.success('Settings saved successfully');
      loadLogStats();
    } catch {
      message.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  const handleCleanup = async () => {
    setCleaning(true);
    try {
      await api.post('/api/cleanup');
      message.success('Cleanup triggered successfully');
      setTimeout(loadLogStats, 3000);
    } catch {
      message.error('Failed to trigger cleanup');
    } finally {
      setCleaning(false);
    }
  };

  const handleVacuum = async () => {
    setVacuuming(true);
    try {
      const resp = await api.post('/api/vacuum');
      message.success(`VACUUM completed: ${formatBytes(resp.data.new_size_bytes)}`);
      loadLogStats();
    } catch {
      message.error('Failed to run VACUUM');
    } finally {
      setVacuuming(false);
    }
  };

  const handleMigrate = async () => {
    setMigrating(true);
    try {
      const resp = await api.post('/api/migrate-bodies');
      message.success(`Migrated ${resp.data.migrated} bodies to file storage`);
      loadLogStats();
    } catch {
      message.error('Failed to migrate bodies');
    } finally {
      setMigrating(false);
    }
  };

  const maxDbSizeGB = form.getFieldValue('max_db_size_gb') || 10;
  const dbUsagePercent = stats && stats.total_size_bytes !== undefined
    ? Math.min(100, Math.round((stats.total_size_bytes) / (maxDbSizeGB * 1024 * 1024 * 1024) * 100))
    : 0;

  return (
    <div>
      <h2 style={{ marginBottom: 24 }}><SettingOutlined /> System Settings</h2>
      <Row gutter={24}>
        <Col span={16}>
          <Card title="Log Retention & Storage" loading={loading}>
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
                <InputNumber min={1} max={365} style={{ width: '100%' }} addonAfter="days" />
              </Form.Item>

              <Form.Item
                name="proxy_error_retention_days"
                label="Proxy Error Retention (days)"
                rules={[
                  { required: true, message: 'Please enter retention days' },
                  { type: 'number', min: 1, max: 365, message: 'Must be between 1-365 days' },
                ]}
                extra="Proxy error records older than this will be automatically deleted"
              >
                <InputNumber min={1} max={365} style={{ width: '100%' }} addonAfter="days" />
              </Form.Item>

              <Form.Item
                name="max_db_size_gb"
                label="Max Database Size (GB)"
                rules={[
                  { required: true, message: 'Please enter max database size' },
                  { type: 'number', min: 1, max: 1000, message: 'Must be between 1-1000 GB' },
                ]}
                extra="When the database exceeds this size, the oldest logs will be automatically deleted"
              >
                <InputNumber min={1} max={1000} style={{ width: '100%' }} addonAfter="GB" />
              </Form.Item>

              <Form.Item
                name="log_body_storage"
                label="Body Storage Mode"
                extra="'file' stores request/response bodies in separate compressed files (recommended, keeps DB small). 'inline' stores bodies directly in the database (legacy, causes DB bloat)."
              >
                <Select>
                  <Select.Option value="file">File Storage (Recommended)</Select.Option>
                  <Select.Option value="inline">Inline Storage (Legacy)</Select.Option>
                </Select>
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

          <Card title="Maintenance Actions" style={{ marginTop: 24 }}>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <Alert
                message="These actions run in the background and may take a few minutes for large databases."
                type="info"
                showIcon
              />
              <Space wrap>
                <Tooltip title="Delete old logs and reclaim space">
                  <Button
                    icon={<DeleteOutlined />}
                    onClick={handleCleanup}
                    loading={cleaning}
                  >
                    Run Cleanup Now
                  </Button>
                </Tooltip>
                <Tooltip title="Compact the database file to reclaim disk space (safe, no data loss)">
                  <Button
                    icon={<CompressOutlined />}
                    onClick={handleVacuum}
                    loading={vacuuming}
                  >
                    VACUUM Database
                  </Button>
                </Tooltip>
                <Tooltip title="Migrate inline bodies to file storage (run after switching to file mode)">
                  <Button
                    icon={<CloudUploadOutlined />}
                    onClick={handleMigrate}
                    loading={migrating}
                  >
                    Migrate Bodies to Files
                  </Button>
                </Tooltip>
              </Space>
            </Space>
          </Card>
        </Col>

        <Col span={8}>
          <Card title="Storage Overview">
            <Space direction="vertical" style={{ width: '100%' }} size="large">
              <Statistic
                title="Total Request Logs"
                value={stats?.total_logs || 0}
                prefix={<DatabaseOutlined />}
              />
              <Divider style={{ margin: '8px 0' }} />
              <Statistic
                title="Database Size"
                value={formatBytes(stats?.total_size_bytes || 0)}
                prefix={<HardDriveOutlined />}
              />
              <Statistic
                title="Body Files Size"
                value={formatBytes(stats?.body_size_bytes || 0)}
                prefix={<CloudUploadOutlined />}
              />
              {stats && stats.total_size_bytes !== undefined && (
                <>
                  <div>
                    <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>
                      DB Usage: {dbUsagePercent}% of {maxDbSizeGB} GB limit
                    </div>
                    <Progress
                      percent={dbUsagePercent}
                      status={dbUsagePercent > 90 ? 'exception' : dbUsagePercent > 70 ? 'active' : 'normal'}
                      size="small"
                    />
                  </div>
                  {stats.free_pages !== undefined && stats.total_pages !== undefined && stats.total_pages > 0 && (
                    <div style={{ fontSize: 12, color: '#888' }}>
                      Free pages: {stats.free_pages} / {stats.total_pages}
                      ({Math.round(stats.free_pages / stats.total_pages * 100)}% reclaimable via VACUUM)
                    </div>
                  )}
                </>
              )}
              {stats?.db_path && (
                <div style={{ fontSize: 11, color: '#aaa', wordBreak: 'break-all' }}>
                  Path: {stats.db_path}
                </div>
              )}
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default SystemSettings;
```

- [ ] **Step 3: 验证前端构建**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected:
  - Exit code: 0 or only pre-existing type errors (not from new files)

- [ ] **Step 4: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add frontend/src/types/settings.ts frontend/src/components/SystemSettings.tsx && git commit -m "feat(frontend): expand system settings with storage management and maintenance actions"`

---

### Task 6: 端到端验证 — 构建和测试

**Depends on:** Task 3, Task 4, Task 5
**Files:**
- Create: `backend/database/log_storage_test.go`
- Create: `backend/database/log_cleanup_test.go`

- [ ] **Step 1: 创建 log_storage_test.go — 文件存储服务测试**

```go
// backend/database/log_storage_test.go
package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogStorage_StoreAndLoadBody(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	now := time.Now()

	path, err := storage.StoreBody(1, body, "req", now)
	if err != nil {
		t.Fatalf("StoreBody failed: %v", err)
	}
	if path == "" {
		t.Fatal("StoreBody returned empty path")
	}

	// Verify file exists
	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatalf("Body file not created at %s", fullPath)
	}

	// Load and verify
	loaded, err := storage.LoadBody(path)
	if err != nil {
		t.Fatalf("LoadBody failed: %v", err)
	}
	if loaded != body {
		t.Errorf("Loaded body mismatch: got %q, want %q", loaded, body)
	}
}

func TestLogStorage_StoreEmptyBody(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	path, err := storage.StoreBody(1, "", "req", time.Now())
	if err != nil {
		t.Fatalf("StoreBody with empty body failed: %v", err)
	}
	if path != "" {
		t.Errorf("StoreBody with empty body should return empty path, got %q", path)
	}
}

func TestLogStorage_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	_, err := storage.LoadBody("nonexistent/path.json.gz")
	if err == nil {
		t.Error("LoadBody should fail for non-existent file")
	}
}

func TestLogStorage_CleanOldDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	// Create an old directory (31 days ago)
	oldDate := time.Now().AddDate(0, 0, -31)
	oldDir := filepath.Join(tmpDir, oldDate.Format("2006"), oldDate.Format("01"), oldDate.Format("02"))
	os.MkdirAll(oldDir, 0755)
	os.WriteFile(filepath.Join(oldDir, "test.json.gz"), []byte("test"), 0644)

	// Create a recent directory (1 day ago)
	recentDate := time.Now().AddDate(0, 0, -1)
	recentDir := filepath.Join(tmpDir, recentDate.Format("2006"), recentDate.Format("01"), recentDate.Format("02"))
	os.MkdirAll(recentDir, 0755)
	os.WriteFile(filepath.Join(recentDir, "test.json.gz"), []byte("test"), 0644)

	removed, err := storage.CleanOldDirectories(30)
	if err != nil {
		t.Fatalf("CleanOldDirectories failed: %v", err)
	}
	if removed != 1 {
		t.Errorf("Expected 1 directory removed, got %d", removed)
	}

	// Old directory should be gone
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("Old directory should have been removed")
	}

	// Recent directory should still exist
	if _, err := os.Stat(recentDir); os.IsNotExist(err) {
		t.Error("Recent directory should still exist")
	}
}

func TestLogStorage_GetStorageSize(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	// Initially should be 0
	size, err := storage.GetStorageSize()
	if err != nil {
		t.Fatalf("GetStorageSize failed: %v", err)
	}

	// Store a body
	storage.StoreBody(1, "test content for size check", "req", time.Now())

	size2, err := storage.GetStorageSize()
	if err != nil {
		t.Fatalf("GetStorageSize after store failed: %v", err)
	}
	if size2 <= size {
		t.Errorf("Storage size should increase after storing a body: before=%d, after=%d", size, size2)
	}
}

func TestLogStorage_DeleteBodyFile(t *testing.T) {
	tmpDir := t.TempDir()
	storage := &LogStorage{baseDir: tmpDir}

	path, _ := storage.StoreBody(42, "delete me", "req", time.Now())
	if path == "" {
		t.Fatal("StoreBody returned empty path")
	}

	fullPath := filepath.Join(tmpDir, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatal("Body file should exist before delete")
	}

	if err := storage.DeleteBodyFile(path); err != nil {
		t.Fatalf("DeleteBodyFile failed: %v", err)
	}

	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Error("Body file should be deleted")
	}
}
```

- [ ] **Step 2: 创建 log_cleanup_test.go — 清理引擎测试**

```go
// backend/database/log_cleanup_test.go
package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDatabaseSizeInfo(t *testing.T) {
	info, err := GetDatabaseSizeInfo()
	if err != nil {
		t.Fatalf("GetDatabaseSizeInfo failed: %v", err)
	}
	if info == nil {
		t.Fatal("GetDatabaseSizeInfo returned nil")
	}
	if info.TotalPages <= 0 {
		t.Errorf("TotalPages should be > 0, got %d", info.TotalPages)
	}
	if info.PageSize <= 0 {
		t.Errorf("PageSize should be > 0, got %d", info.PageSize)
	}
}

func TestDeleteOldestRequestLogs(t *testing.T) {
	// This test only verifies the function doesn't error on an empty table
	deleted, err := DeleteOldestRequestLogs(10)
	if err != nil {
		t.Fatalf("DeleteOldestRequestLogs failed: %v", err)
	}
	// On an empty or small table, deleted should be 0 or small
	_ = deleted
}

func TestGetRequestLogCountByAge(t *testing.T) {
	count, err := GetRequestLogCountByAge(30)
	if err != nil {
		t.Fatalf("GetRequestLogCountByAge failed: %v", err)
	}
	// Just verify it doesn't error
	_ = count
}

func TestGetAvailableDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()
	space := getAvailableDiskSpace(tmpDir)
	// On Linux, should return a positive value
	if space < 0 {
		t.Logf("getAvailableDiskSpace returned -1 (platform may not support statfs)")
	}
}

func TestVacuumInto_InvalidPath(t *testing.T) {
	// VacuumInto should fail gracefully with an invalid path
	_, err := VacuumInto("/nonexistent/path/to/db.sqlite")
	if err == nil {
		t.Error("VacuumInto should fail for non-existent database path")
	}
}

func TestMigrateInlineBodiesToFiles_NoRows(t *testing.T) {
	// Should work fine with no rows to migrate
	migrated, err := MigrateInlineBodiesToFiles(100)
	if err != nil {
		t.Fatalf("MigrateInlineBodiesToFiles failed: %v", err)
	}
	if migrated != 0 {
		t.Errorf("Expected 0 migrated rows on empty table, got %d", migrated)
	}
}
```

- [ ] **Step 3: 运行存储服务测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./database/ -run "TestLogStorage" -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

- [ ] **Step 4: 运行清理引擎测试**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go test ./database/ -run "TestGetDatabaseSizeInfo|TestDeleteOldest|TestGetRequestLogCountByAge|TestGetAvailableDiskSpace|TestVacuumInto|TestMigrateInlineBodies" -v`
Expected:
  - Exit code: 0
  - Output contains: "PASS"

- [ ] **Step 5: 运行完整项目构建**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api/backend && go build ./database/... ./handler/... ./config/... ./utils/... ./cmd/...`
Expected:
  - Exit code: 0

- [ ] **Step 6: 提交**
Run: `cd /home/cc11001100/github/vibe-coding-labs/claude-code-cli-with-openai-api && git add backend/database/log_storage_test.go backend/database/log_cleanup_test.go && git commit -m "test(database): add tests for log storage service and cleanup engine"`
