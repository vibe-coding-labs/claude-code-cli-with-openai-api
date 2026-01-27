import { test, expect } from '@playwright/test';

test.describe('Load Balancer Management', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
  });

  test('T019: Load Balancer List View', async ({ page }) => {
    // Navigate to load balancers page
    await page.goto('/ui/load-balancers');
    
    // Verify load balancer list is displayed
    const table = await page.locator('table').first();
    await expect(table).toBeVisible();
    
    // Verify columns
    await expect(page.locator('th:has-text("Name")').or(page.locator('th:has-text("名称")))).toBeVisible();
    await expect(page.locator('th:has-text("Strategy")').or(page.locator('th:has-text("策略")))).toBeVisible();
    await expect(page.locator('th:has-text("Nodes")').or(page.locator('th:has-text("节点")))).toBeVisible();
    
    // Verify "Create Load Balancer" button
    await expect(page.locator('button:has-text("Create Load Balancer"), button:has-text("创建负载均衡器")')).toBeVisible();
  });

  test('T020: Create Load Balancer - Round Robin', async ({ page }) => {
    // Navigate to load balancers page
    await page.goto('/ui/load-balancers');
    
    // Click "Create Load Balancer" button
    await page.click('button:has-text("Create Load Balancer"), button:has-text("创建负载均衡器")');
    
    // Wait for create page to load
    await page.waitForURL('**/load-balancers/create');
    
    // Fill in load balancer details
    await page.fill('input[placeholder*="name" i], input[name*="name"]', 'Test LB 1');
    
    // Select round-robin strategy
    const strategySelect = page.locator('select, .ant-select').first();
    await strategySelect.click();
    await page.click('li:has-text("Round Robin"), li:has-text("轮询")');
    
    // Add config nodes (simplified - may need adjustment based on actual UI)
    const addNodeButton = page.locator('button:has-text("Add Node"), button:has-text("添加节点")').first();
    if (await addNodeButton.count() > 0) {
      await addNodeButton.click();
      await page.waitForTimeout(500);
      await addNodeButton.click();
      await page.waitForTimeout(500);
    }
    
    // Configure health check settings
    const healthCheckEnabled = page.locator('input[type="checkbox"]').first();
    if (await healthCheckEnabled.count() > 0 && !(await healthCheckEnabled.isChecked())) {
      await healthCheckEnabled.check();
    }
    
    // Click save
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Verify success message
    const success = await page.locator('.ant-message-success, .success-message').first();
    await expect(success).toBeVisible();
    
    // Verify load balancer appears in list
    await page.goto('/ui/load-balancers');
    await expect(page.locator('td:has-text("Test LB 1")').first()).toBeVisible();
  });

  test('T021: Create Load Balancer - Weighted', async ({ page }) => {
    // Navigate to create load balancer page
    await page.goto('/ui/load-balancers/create');
    
    // Fill in load balancer details
    await page.fill('input[placeholder*="name" i], input[name*="name"]', 'Test LB 2');
    
    // Select weighted round-robin strategy
    const strategySelect = page.locator('select, .ant-select').first();
    await strategySelect.click();
    await page.click('li:has-text("Weighted"), li:has-text("加权")');
    
    // Add config nodes with different weights
    const addNodeButton = page.locator('button:has-text("Add Node"), button:has-text("添加节点")').first();
    if (await addNodeButton.count() > 0) {
      await addNodeButton.click();
      await page.waitForTimeout(500);
      
      // Set weight for first node
      const weightInput = page.locator('input[type="number"], input[placeholder*="weight"]').first();
      if (await weightInput.count() > 0) {
        await weightInput.fill('10');
      }
      
      await addNodeButton.click();
      await page.waitForTimeout(500);
      
      // Set weight for second node
      const weightInputs = page.locator('input[type="number"], input[placeholder*="weight"]').all();
      if (weightInputs.length > 1) {
        await weightInputs[1].fill('5');
      }
    }
    
    // Click save
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Verify success message
    const success = await page.locator('.ant-message-success, .success-message').first();
    await expect(success).toBeVisible();
    
    // Verify load balancer is created with weights
    await page.goto('/ui/load-balancers');
    await expect(page.locator('td:has-text("Test LB 2")').first()).toBeVisible();
  });

  test('T022: Edit Load Balancer Configuration', async ({ page }) => {
    // Navigate to load balancers page
    await page.goto('/ui/load-balancers');
    
    // Find first load balancer and click edit
    const editButton = page.locator('button:has-text("Edit"), button:has-text("编辑")').first();
    if (await editButton.count() > 0) {
      await editButton.click();
      
      // Wait for edit page to load
      await page.waitForTimeout(1000);
      
      // Modify strategy or health check settings
      const strategySelect = page.locator('select, .ant-select').first();
      await strategySelect.click();
      await page.click('li:has-text("Random"), li:has-text("随机")');
      
      // Click save
      await page.click('button:has-text("Save"), button:has-text("保存")');
      
      // Verify success message
      const success = await page.locator('.ant-message-success, .success-message').first();
      await expect(success).toBeVisible();
    }
  });

  test('T023: View Load Balancer Details', async ({ page }) => {
    // Navigate to load balancers page
    await page.goto('/ui/load-balancers');
    
    // Click on first load balancer name
    const lbName = page.locator('td:has-text("LB"), td:has-text("Test")').first();
    if (await lbName.count() > 0) {
      await lbName.click();
      
      // Verify overview section is displayed
      await expect(page.locator('h1, h2, h3').filter({ hasText: /Details|详情/i })).toBeVisible();
      
      // Verify health status panel shows node health
      await expect(page.locator('text=/health|健康/i').or(page.locator('.health-status'))).toBeVisible();
      
      // Verify circuit breaker status is visible
      await expect(page.locator('text=/circuit|熔断/i').or(page.locator('.circuit-breaker'))).toBeVisible();
      
      // Verify statistics charts are displayed
      await expect(page.locator('.chart, canvas, svg').first()).toBeVisible();
    }
  });

  test('T024: Load Balancer Health Check Monitoring', async ({ page }) => {
    // Navigate to load balancer details
    await page.goto('/ui/load-balancers');
    
    const lbName = page.locator('td:has-text("LB"), td:has-text("Test")').first();
    if (await lbName.count() > 0) {
      await lbName.click();
      
      // Observe health status panel
      const healthPanel = page.locator('.health-status, [class*="health"]').first();
      await expect(healthPanel).toBeVisible();
      
      // Verify node health indicators
      await expect(page.locator('text=/Healthy|Unhealthy|健康|不健康/i')).toBeVisible();
      
      // Verify health check timestamps
      await expect(page.locator('text=/timestamp|时间/i').or(page.locator('[class*="time"]'))).toBeVisible();
    }
  });

  test('T025: Load Balancer Circuit Breaker Status', async ({ page }) => {
    // Navigate to load balancer details
    await page.goto('/ui/load-balancers');
    
    const lbName = page.locator('td:has-text("LB"), td:has-text("Test")').first();
    if (await lbName.count() > 0) {
      await lbName.click();
      
      // View circuit breaker panel
      const circuitPanel = page.locator('.circuit-breaker, [class*="circuit"]').first();
      await expect(circuitPanel).toBeVisible();
      
      // Verify state is displayed
      await expect(page.locator('text=/Closed|Open|Half-Open|关闭|打开|半开/i')).toBeVisible();
      
      // Verify error rate is shown
      await expect(page.locator('text=/error rate|错误率/i')).toBeVisible();
      
      // Verify circuit breaker metrics
      await expect(page.locator('text=/threshold|阈值/i').or(page.locator('text=/window|窗口/i'))).toBeVisible();
    }
  });

  test('T026: Delete Load Balancer', async ({ page }) => {
    // Navigate to load balancers page
    await page.goto('/ui/load-balancers');
    
    // Find first load balancer and click delete
    const deleteButton = page.locator('button:has-text("Delete"), button:has-text("删除")').first();
    if (await deleteButton.count() > 0) {
      // Get LB name before deletion
      const lbName = await deleteButton.locator('xpath=ancestor::tr').locator('td').first().textContent();
      
      await deleteButton.click();
      
      // Confirm deletion in modal
      await page.click('button:has-text("OK"), button:has-text("确认"), button:has-text("Yes")');
      
      // Verify success message
      const success = await page.locator('.ant-message-success, .success-message').first();
      await expect(success).toBeVisible();
      
      // Verify load balancer is removed from list
      await page.waitForTimeout(1000);
      await expect(page.locator(`td:has-text("${lbName}")`).first()).not.toBeVisible();
    }
  });
});
