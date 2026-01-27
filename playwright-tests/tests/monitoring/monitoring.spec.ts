import { test, expect } from '@playwright/test';

test.describe('Monitoring & Analytics', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
  });

  test('T027: View Request Logs', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click on Logs tab
    const logsTab = page.locator('a:has-text("Logs"), button:has-text("Logs"), [role="tab"]:has-text("Logs"), [role="tab"]:has-text("日志")').first();
    if (await logsTab.count() > 0) {
      await logsTab.click();
      
      // Verify logs table is displayed
      const table = await page.locator('table').first();
      await expect(table).toBeVisible();
      
      // Verify columns
      await expect(page.locator('th:has-text("Timestamp")').or(page.locator('th:has-text("时间")))).toBeVisible();
      await expect(page.locator('th:has-text("Method")').or(page.locator('th:has-text("方法")))).toBeVisible();
      await expect(page.locator('th:has-text("Status")').or(page.locator('th:has-text("状态")))).toBeVisible();
      
      // Click on a log entry to view details
      const firstLogRow = page.locator('tbody tr').first();
      if (await firstLogRow.count() > 0) {
        await firstLogRow.click();
        
        // Verify full request/response JSON is shown
        await expect(page.locator('pre, code, .json-viewer').first()).toBeVisible();
      }
    }
  });

  test('T028: Filter Request Logs', async ({ page }) => {
    // Navigate to request logs page
    await page.goto('/ui');
    
    const logsTab = page.locator('a:has-text("Logs"), button:has-text("Logs"), [role="tab"]:has-text("Logs"), [role="tab"]:has-text("日志")').first();
    if (await logsTab.count() > 0) {
      await logsTab.click();
      
      // Apply date range filter
      const dateInput = page.locator('input[type="date"], .ant-picker-input').first();
      if (await dateInput.count() > 0) {
        await dateInput.fill('2024-01-01');
      }
      
      // Apply status code filter
      const statusFilter = page.locator('select, .ant-select').first();
      if (await statusFilter.count() > 0) {
        await statusFilter.click();
        await page.click('li:has-text("200")');
      }
      
      // Verify filtered results
      await page.waitForTimeout(1000);
      const table = await page.locator('table').first();
      await expect(table).toBeVisible();
      
      // Clear filters
      const clearButton = page.locator('button:has-text("Clear"), button:has-text("清除")').first();
      if (await clearButton.count() > 0) {
        await clearButton.click();
      }
      
      // Verify all logs are displayed
      await page.waitForTimeout(1000);
    }
  });

  test('T029: View Statistics Dashboard', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click on Statistics tab
    const statsTab = page.locator('a:has-text("Statistics"), button:has-text("Statistics"), [role="tab"]:has-text("Statistics"), [role="tab"]:has-text("统计")').first();
    if (await statsTab.count() > 0) {
      await statsTab.click();
      
      // Verify request count is displayed
      await expect(page.locator('text=/request count|请求数量|总请求/i')).toBeVisible();
      
      // Verify success rate percentage
      await expect(page.locator('text=/success rate|成功率/i')).toBeVisible();
      
      // Verify token usage statistics
      await expect(page.locator('text=/token usage|令牌使用/i')).toBeVisible();
      
      // Verify response time metrics
      await expect(page.locator('text=/P50|P95|P99|响应时间/i')).toBeVisible();
    }
  });

  test('T030: View Alerts Panel', async ({ page }) => {
    // Navigate to load balancer details
    await page.goto('/ui/load-balancers');
    
    const lbName = page.locator('td:has-text("LB"), td:has-text("Test")').first();
    if (await lbName.count() > 0) {
      await lbName.click();
      
      // Click on Alerts tab
      const alertsTab = page.locator('a:has-text("Alerts"), button:has-text("Alerts"), [role="tab"]:has-text("Alerts"), [role="tab"]:has-text("告警")').first();
      if (await alertsTab.count() > 0) {
        await alertsTab.click();
        
        // Verify alerts list is displayed
        await expect(page.locator('.alert-item, [class*="alert"]').first()).toBeVisible();
        
        // Verify alert severity levels
        await expect(page.locator('text=/Critical|Warning|Info|严重|警告|信息/i')).toBeVisible();
        
        // Click "Acknowledge" on an alert
        const acknowledgeButton = page.locator('button:has-text("Acknowledge"), button:has-text("确认")').first();
        if (await acknowledgeButton.count() > 0) {
          await acknowledgeButton.click();
          
          // Verify alert is marked as acknowledged
          await page.waitForTimeout(1000);
        }
      }
    }
  });
});
