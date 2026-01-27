import { test, expect } from '@playwright/test';

test.describe('API Configuration Management', () => {
  test.beforeEach(async ({ page }) => {
    // Login before each test
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
  });

  test('T009: API Config List View', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Verify config list table is displayed
    const table = await page.locator('table').first();
    await expect(table).toBeVisible();
    
    // Verify columns
    await expect(page.locator('th:has-text("Name")').or(page.locator('th:has-text("名称")))).toBeVisible();
    await expect(page.locator('th:has-text("API Key")').or(page.locator('th:has-text("API密钥")))).toBeVisible();
    await expect(page.locator('th:has-text("Base URL")').or(page.locator('th:has-text("基础URL")))).toBeVisible();
    
    // Verify "Create Config" button is visible
    await expect(page.locator('button:has-text("Create Config"), button:has-text("创建配置")')).toBeVisible();
  });

  test('T010: Create API Configuration - Basic', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click "Create Config" button
    await page.click('button:has-text("Create Config"), button:has-text("创建配置")');
    
    // Wait for create page to load
    await page.waitForURL('**/configs/create');
    
    // Fill in configuration details
    await page.fill('input[placeholder*="name" i], input[name*="name"]', 'Test Config 1');
    await page.fill('input[placeholder*="api key" i], input[name*="apiKey"]', 'sk-test-xxxxxxxxxxxx');
    await page.fill('input[placeholder*="base url" i], input[name*="baseUrl"]', 'https://api.openai.com/v1');
    
    // Select models
    const modelInput = page.locator('input[placeholder*="model" i]').first();
    if (await modelInput.count() > 0) {
      await modelInput.fill('gpt-4o');
      await page.keyboard.press('Enter');
      await modelInput.fill('gpt-4o-mini');
      await page.keyboard.press('Enter');
    }
    
    // Click save
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Verify success message
    const success = await page.locator('.ant-message-success, .success-message').first();
    await expect(success).toBeVisible();
    
    // Verify config appears in list
    await expect(page.locator('td:has-text("Test Config 1")').first()).toBeVisible();
  });

  test('T011: Create API Configuration - Validation', async ({ page }) => {
    // Navigate to create config page
    await page.goto('/ui/configs/create');
    
    // Click save without filling required fields
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Verify validation errors for required fields
    const errors = await page.locator('.ant-form-item-explain-error, .error-message').all();
    expect(errors.length).toBeGreaterThan(0);
    
    // Enter invalid URL format
    await page.fill('input[placeholder*="base url" i], input[name*="baseUrl"]', 'invalid-url');
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Verify URL validation error
    await expect(page.locator('.ant-form-item-explain-error:has-text("URL"), .error-message:has-text("URL")')).toBeVisible();
    
    // Verify form cannot be submitted with invalid data
    await page.waitForTimeout(1000);
    expect(page.url()).toContain('/create');
  });

  test('T012: Edit API Configuration', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Find first config and click edit
    const editButton = page.locator('button:has-text("Edit"), button:has-text("编辑")').first();
    if (await editButton.count() > 0) {
      await editButton.click();
      
      // Wait for edit page to load
      await page.waitForTimeout(1000);
      
      // Modify name
      const nameInput = page.locator('input[placeholder*="name" i], input[name*="name"]').first();
      const currentName = await nameInput.inputValue();
      await nameInput.fill(`${currentName} - Edited`);
      
      // Click save
      await page.click('button:has-text("Save"), button:has-text("保存")');
      
      // Verify success message
      const success = await page.locator('.ant-message-success, .success-message').first();
      await expect(success).toBeVisible();
      
      // Verify changes are reflected in list
      await page.goto('/ui');
      await expect(page.locator(`td:has-text("${currentName} - Edited")`).first()).toBeVisible();
    }
  });

  test('T013: Delete API Configuration', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Find first config and click delete
    const deleteButton = page.locator('button:has-text("Delete"), button:has-text("删除")').first();
    if (await deleteButton.count() > 0) {
      // Get config name before deletion
      const configName = await deleteButton.locator('xpath=ancestor::tr').locator('td').first().textContent();
      
      await deleteButton.click();
      
      // Confirm deletion in modal
      await page.click('button:has-text("OK"), button:has-text("确认"), button:has-text("Yes")');
      
      // Verify success message
      const success = await page.locator('.ant-message-success, .success-message').first();
      await expect(success).toBeVisible();
      
      // Verify config is removed from list
      await page.waitForTimeout(1000);
      await expect(page.locator(`td:has-text("${configName}")`).first()).not.toBeVisible();
    }
  });

  test('T014: Renew API Key', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Find first config and click renew key
    const renewButton = page.locator('button:has-text("Renew Key"), button:has-text("更新密钥")').first();
    if (await renewButton.count() > 0) {
      await renewButton.click();
      
      // Confirm action in modal
      await page.click('button:has-text("OK"), button:has-text("确认"), button:has-text("Yes")');
      
      // Verify success message
      const success = await page.locator('.ant-message-success, .success-message').first();
      await expect(success).toBeVisible();
      
      // Verify new API key is displayed
      await page.waitForTimeout(1000);
      const apiKey = await page.locator('td:has-text("sk-")').first();
      await expect(apiKey).toBeVisible();
    }
  });

  test('T015: Enable/Disable Configuration', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Find first config and toggle enable/disable
    const toggleButton = page.locator('button:has-text("Enable"), button:has-text("Disable"), button:has-text("启用"), button:has-text("禁用")').first();
    if (await toggleButton.count() > 0) {
      const buttonText = await toggleButton.textContent();
      
      await toggleButton.click();
      
      // Verify success message
      const success = await page.locator('.ant-message-success, .success-message').first();
      await expect(success).toBeVisible();
      
      // Verify status changed
      await page.waitForTimeout(1000);
      const newButtonText = await toggleButton.textContent();
      expect(newButtonText).not.toBe(buttonText);
    }
  });

  test('T016: View Configuration Details', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click on first config name to view details
    const configName = page.locator('td:has-text("Test"), td:has-text("Config")').first();
    if (await configName.count() > 0) {
      await configName.click();
      
      // Verify all config fields are displayed
      await expect(page.locator('h1, h2, h3').filter({ hasText: /Details|详情/i })).toBeVisible();
      
      // Verify Anthropic API key is shown
      await expect(page.locator('input:has-text("sk-")').or(page.locator('div:has-text("sk-")'))).toBeVisible();
      
      // Verify usage statistics are visible
      await expect(page.locator('text=/requests|请求/i').or(page.locator('text=/tokens|令牌/i'))).toBeVisible();
    }
  });

  test('T017: Test API Configuration - Basic', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click on test button for first config
    const testButton = page.locator('button:has-text("Test"), button:has-text("测试")').first();
    if (await testButton.count() > 0) {
      await testButton.click();
      
      // Wait for test page to load
      await page.waitForTimeout(1000);
      
      // Enter test message
      await page.fill('textarea[placeholder*="message" i], textarea[name*="message"]', 'Hello, world!');
      
      // Select model
      const modelSelect = page.locator('select, .ant-select').first();
      if (await modelSelect.count() > 0) {
        await modelSelect.click();
        await page.click('li:has-text("gpt-4o")');
      }
      
      // Set max tokens
      const tokenInput = page.locator('input[placeholder*="tokens" i], input[name*="maxTokens"]').first();
      if (await tokenInput.count() > 0) {
        await tokenInput.fill('100');
      }
      
      // Click send test
      await page.click('button:has-text("Send Test"), button:has-text("发送测试")');
      
      // Wait for response
      await page.waitForTimeout(5000);
      
      // Verify response is displayed
      await expect(page.locator('.response-content, .test-response').first()).toBeVisible();
      
      // Verify token statistics
      await expect(page.locator('text=/tokens|令牌/i')).toBeVisible();
    }
  });

  test('T018: Test API Configuration - Streaming', async ({ page }) => {
    // Navigate to configs page
    await page.goto('/ui');
    
    // Click on test button for first config
    const testButton = page.locator('button:has-text("Test"), button:has-text("测试")').first();
    if (await testButton.count() > 0) {
      await testButton.click();
      
      // Wait for test page to load
      await page.waitForTimeout(1000);
      
      // Enable streaming option
      const streamCheckbox = page.locator('input[type="checkbox"]').first();
      if (await streamCheckbox.count() > 0 && !(await streamCheckbox.isChecked())) {
        await streamCheckbox.check();
      }
      
      // Enter test message
      await page.fill('textarea[placeholder*="message" i], textarea[name*="message"]', 'Hello, world!');
      
      // Click send test
      await page.click('button:has-text("Send Test"), button:has-text("发送测试")');
      
      // Wait for streaming response
      await page.waitForTimeout(3000);
      
      // Verify streaming response is displayed
      await expect(page.locator('.response-content, .test-response').first()).toBeVisible();
    }
  });
});
