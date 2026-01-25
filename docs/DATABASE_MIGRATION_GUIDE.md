# 数据库迁移指南

## 概述

本指南介绍如何管理和执行数据库迁移，以支持负载均衡器增强功能。

## 迁移脚本

所有迁移脚本位于 `database/migrations/` 目录：

1. **001_create_health_statuses_table.sql** - 创建健康状态表
2. **002_create_circuit_breaker_states_table.sql** - 创建熔断器状态表
3. **003_create_lb_request_logs_table.sql** - 创建请求日志表
4. **004_create_lb_stats_tables.sql** - 创建统计数据表
5. **005_create_alerts_table.sql** - 创建告警表
6. **006_add_lb_enhancement_config_fields.sql** - 扩展负载均衡器配置表

## 执行迁移

### 方式一：使用命令行工具（推荐）

```bash
# 查看迁移状态
./claude-code-cli-with-openai-api migrate status

# 执行所有待执行的迁移
./claude-code-cli-with-openai-api migrate up

# 回滚迁移（需要手动执行）
./claude-code-cli-with-openai-api migrate down <version>
```

### 方式二：自动迁移

服务启动时会自动执行待执行的迁移：

```bash
./claude-code-cli-with-openai-api server
```

### 方式三：手动执行SQL

```bash
# 连接到数据库
sqlite3 data/proxy.db

# 执行迁移脚本
.read database/migrations/001_create_health_statuses_table.sql
```

## 迁移状态

### 查看已应用的迁移

```bash
./claude-code-cli-with-openai-api migrate status
```

输出示例：
```
Migration Status:
Version  Name                                    Status      Applied At
-------  --------------------------------------  ----------  -------------------
001      create_health_statuses_table            Applied     2024-01-15 10:30:00
002      create_circuit_breaker_states_table     Applied     2024-01-15 10:30:01
003      create_lb_request_logs_table            Applied     2024-01-15 10:30:02
004      create_lb_stats_tables                  Applied     2024-01-15 10:30:03
005      create_alerts_table                     Applied     2024-01-15 10:30:04
006      add_lb_enhancement_config_fields        Pending     -
```

### 查看迁移历史

```bash
sqlite3 data/proxy.db "SELECT * FROM schema_migrations ORDER BY version"
```

## 生产环境迁移

### 升级前准备

1. **备份数据库**：
```bash
# 创建备份
cp data/proxy.db data/proxy.db.backup.$(date +%Y%m%d_%H%M%S)

# 或使用SQLite备份命令
sqlite3 data/proxy.db ".backup data/proxy.db.backup"
```

2. **测试迁移**：
```bash
# 在测试环境执行迁移
./claude-code-cli-with-openai-api migrate up

# 验证数据完整性
sqlite3 data/proxy.db ".schema"
```

3. **准备回滚方案**：
```bash
# 记录当前迁移版本
./claude-code-cli-with-openai-api migrate status > migration_status_before.txt
```

### 执行迁移

1. **停止服务**：
```bash
# 优雅关闭
kill -SIGTERM <pid>

# 等待所有请求完成
```

2. **执行迁移**：
```bash
./claude-code-cli-with-openai-api migrate up
```

3. **验证迁移**：
```bash
# 检查迁移状态
./claude-code-cli-with-openai-api migrate status

# 验证表结构
sqlite3 data/proxy.db ".schema load_balancers"
```

4. **启动服务**：
```bash
./claude-code-cli-with-openai-api server
```

5. **验证功能**：
```bash
# 测试健康检查
curl http://localhost:8080/health

# 测试负载均衡器API
curl http://localhost:8080/api/load-balancers
```

### 回滚迁移

如果迁移失败或需要回滚：

1. **停止服务**：
```bash
kill -SIGTERM <pid>
```

2. **恢复数据库**：
```bash
cp data/proxy.db.backup data/proxy.db
```

3. **启动服务**：
```bash
./claude-code-cli-with-openai-api server
```

## 零停机迁移

对于需要零停机的场景，可以使用以下策略：

### 1. 蓝绿部署

1. 部署新版本到备用环境
2. 执行数据库迁移
3. 切换流量到新环境
4. 验证功能正常
5. 关闭旧环境

### 2. 滚动更新

1. 确保迁移向后兼容
2. 先执行数据库迁移
3. 逐步更新应用实例
4. 验证每个实例的功能

### 3. 在线迁移

对于大表的迁移，可以使用在线迁移工具：

```bash
# 使用pt-online-schema-change（MySQL）
# 或其他在线迁移工具
```

## 迁移最佳实践

### 1. 迁移脚本编写

- **幂等性**：迁移脚本应该可以重复执行
- **向后兼容**：新版本应该兼容旧数据
- **事务性**：使用事务确保原子性
- **测试**：在测试环境充分测试

### 2. 迁移命名

- 使用序号前缀：`001_`, `002_`, ...
- 使用描述性名称：`create_xxx_table`, `add_xxx_column`
- 使用下划线分隔：`001_create_health_statuses_table.sql`

### 3. 迁移内容

- **UP部分**：应用迁移的SQL
- **DOWN部分**：回滚迁移的SQL（注释）
- **注释**：说明迁移的目的和影响

### 4. 数据迁移

对于数据迁移：

```sql
-- 示例：迁移旧数据到新表
INSERT INTO new_table (id, name, value)
SELECT id, name, value FROM old_table;

-- 验证数据
SELECT COUNT(*) FROM new_table;
SELECT COUNT(*) FROM old_table;
```

### 5. 索引创建

对于大表，分步创建索引：

```sql
-- 先创建表
CREATE TABLE large_table (...);

-- 插入数据
INSERT INTO large_table ...;

-- 最后创建索引
CREATE INDEX idx_large_table_column ON large_table(column);
```

## 故障排查

### 迁移失败

**症状**：迁移执行失败，服务无法启动

**排查步骤**：

1. 查看错误日志：
```bash
tail -f server.log | grep migration
```

2. 检查迁移状态：
```bash
./claude-code-cli-with-openai-api migrate status
```

3. 检查数据库完整性：
```bash
sqlite3 data/proxy.db "PRAGMA integrity_check"
```

**解决方案**：

- 如果是SQL错误，修复迁移脚本后重新执行
- 如果是数据问题，恢复备份后重新迁移
- 如果是版本冲突，手动调整schema_migrations表

### 迁移卡住

**症状**：迁移执行时间过长

**排查步骤**：

1. 检查数据库锁：
```bash
sqlite3 data/proxy.db "PRAGMA busy_timeout"
```

2. 检查进程状态：
```bash
ps aux | grep claude-code-cli-with-openai-api
```

**解决方案**：

- 停止其他访问数据库的进程
- 增加超时时间
- 分批执行大数据量迁移

### 数据不一致

**症状**：迁移后数据不一致

**排查步骤**：

1. 检查数据完整性：
```bash
sqlite3 data/proxy.db "SELECT COUNT(*) FROM table_name"
```

2. 比较迁移前后的数据：
```bash
# 导出迁移前的数据
sqlite3 data/proxy.db.backup ".dump table_name" > before.sql

# 导出迁移后的数据
sqlite3 data/proxy.db ".dump table_name" > after.sql

# 比较差异
diff before.sql after.sql
```

**解决方案**：

- 恢复备份
- 修复迁移脚本
- 重新执行迁移

## 监控和告警

### 迁移监控

在生产环境执行迁移时，应该监控：

- 迁移执行时间
- 数据库大小变化
- 服务可用性
- 错误日志

### 告警设置

设置以下告警：

- 迁移执行时间超过阈值
- 迁移失败
- 数据库空间不足
- 服务不可用

## 参考资料

- [SQLite Migration Best Practices](https://www.sqlite.org/lang_altertable.html)
- [Database Migration Patterns](https://martinfowler.com/articles/evodb.html)
- [Zero-Downtime Migrations](https://www.braintreepayments.com/blog/safe-operations-for-high-volume-postgresql/)

## 附录

### 迁移脚本模板

```sql
-- Migration: XXX_description
-- Description: Brief description of what this migration does
-- Date: YYYY-MM-DD

-- ============================================================================
-- UP Migration
-- ============================================================================

-- Your migration SQL here
CREATE TABLE IF NOT EXISTS example_table (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_example_table_name ON example_table(name);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================

-- DROP TABLE example_table;
-- Note: SQLite has limited ALTER TABLE support
-- For complex rollbacks, you may need to:
-- 1. Create a new table without the changes
-- 2. Copy data from old table to new table
-- 3. Drop old table
-- 4. Rename new table to old name
```

### 常用SQL命令

```sql
-- 查看所有表
.tables

-- 查看表结构
.schema table_name

-- 查看索引
.indexes table_name

-- 导出数据
.dump table_name

-- 导入数据
.read file.sql

-- 检查完整性
PRAGMA integrity_check;

-- 查看数据库大小
.dbinfo

-- 优化数据库
VACUUM;

-- 分析查询计划
EXPLAIN QUERY PLAN SELECT * FROM table_name;
```
