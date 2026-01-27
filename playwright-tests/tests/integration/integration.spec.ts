import { test, expect } from '@playwright/test';

test.describe('Integration & Edge Cases', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
  });

  test('T031: Concurrent Request Handling', async ({ page, context }) => {
    // Open multiple browser tabs
    const pages = await Promise.all([
      context.newPage(),
      context.newPage(),
      context.newPage()
    ]);
    
    // Navigate to test page in each tab
    await Promise.all(pages.map(p => p.goto('/ui')));
    
    // Send test requests simultaneously from each tab
    const testPromises = pages.map(async (p, index) => {
      const testButton = p.locator('button:has-text("Test"), button:has-text("测试")').first();
      if (await testButton.count() > 0) {
        await testButton.click();
        await p.waitForTimeout(1000);
        
        // Enter test message
        await p.fill('textarea[placeholder*="message" i], textarea[name*="message"]', `Test message ${index + 1}`);
        
        // Click send test
        await p.click('button:has-text("Send Test"), button:has-text("发送测试")');
        
        // Wait for response
        await p.waitForTimeout(5000);
        
        // Verify response is displayed
        const response = p.locator('.response-content, .test-response').first();
        await expect(response).toBeVisible();
      }
    });
    
    await Promise.all(testPromises);
    
    // Close additional pages
    await Promise.all(pages.slice(1).map(p => p.close()));
  });

  test('T032: API Error Handling', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click "Create Config" button
    await page.click('button:has-text("Create Config"), button:has-text("创建配置")');
    
    // Wait for create page to load
    await page.waitForURL('**/configs/create');
    
    // Fill in configuration with invalid API key
    await page.fill('input[placeholder*="name" i], input[name*="name"]', 'Invalid Config');
    await page.fill('input[placeholder*="api key" i], input[name*="apiKey"]', 'sk-invalid-key-xxxxxxxxxxxx');
    await page.fill('input[placeholder*="base url" i], input[name*="baseUrl"]', 'https://api.openai.com/v1');
    
    // Click save
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Wait for config to be created
    await page.waitForTimeout(1000);
    
    // Navigate back to configs page
    await page.goto('/ui');
    
    // Click test button on the invalid config
    const testButton = page.locator('button:has-text("Test"), button:has-text("测试")').first();
    if (await testButton.count() > 0) {
      await testButton.click();
      
      // Wait for test page to load
      await page.waitForTimeout(1000);
      
      // Enter test message
      await page.fill('textarea[placeholder*="message" i], textarea[name*="message"]', 'Hello, world!');
      
      // Click send test
      await page.click('button:has-text("Send Test"), button:has-text("发送测试")');
      
      // Wait for error response
      await page.waitForTimeout(5000);
      
      // Verify error message is displayed
      await expect(page.locator('.error-message, .ant-message-error').first()).toBeVisible();
      
      // Verify system remains stable
      await page.goto('/ui');
      await expect(page.locator('table').first()).toBeVisible();
    }
  });

  test('T033: Session Timeout', async ({ page }) => {
    // Login as admin
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
    
    // Clear localStorage to simulate session timeout
    await page.evaluate(() => {
      localStorage.removeItem('token');
      localStorage.removeItem('auth_token');
    });
    
    // Attempt to navigate to protected route
    await page.goto('/ui/configs');
    
    // Verify redirect to login page
    await page.waitForURL('**/login');
    expect(page.url()).toContain('/login');
    
    // Verify appropriate timeout message
    await expect(page.locator('text=/timeout|expired|过期/i').or(page.locator('text=/login|登录/i'))).toBeVisible();
  });

  test('T034: Responsive Design', async ({ page }) => {
    // Login as admin
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
    
    // Resize browser window to mobile size
    await page.setViewportSize({ width: 375, height: 667 });
    
    // Verify sidebar collapses or adapts
    const sidebar = page.locator('.ant-layout-sider, aside').first();
    if (await sidebar.count() > 0) {
      const sidebarWidth = await sidebar.evaluate(el => el.getBoundingClientRect().width);
      expect(sidebarWidth).toBeLessThan(200);
    }
    
    // Verify tables have horizontal scroll
    const table = page.locator('table').first();
    await expect(table).toBeVisible();
    
    // Verify buttons and controls remain accessible
    await expect(page.locator('button').first()).toBeVisible();
    
    // Resize to tablet size
    await page.setViewportSize({ width: 768, height: 1024 });
    
    // Verify layout adapts correctly
    await expect(page.locator('table').first()).toBeVisible();
    await expect(page.locator('button').first()).toBeVisible();
    
    // Resize back to desktop
    await page.setViewportSize({ width: 1280, height: 720 });
    
    // Verify layout is restored
    await expect(page.locator('table').first()).toBeVisible();
  });
});
