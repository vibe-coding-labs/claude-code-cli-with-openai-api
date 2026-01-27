import { test, expect } from '@playwright/test';

test.describe('Authentication & User Management', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to base URL
    await page.goto('/');
  });

  test('T001: User Initialization', async ({ page }) => {
    // Check if we need to initialize (redirected to /ui/initialize)
    const currentUrl = page.url();
    
    if (currentUrl.includes('/initialize')) {
      // Fill in initialization form
      await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
      await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
      
      // Click initialize button
      await page.click('button:has-text("Initialize"), button[type="submit"]');
      
      // Verify redirect to login
      await page.waitForURL('**/login');
      expect(page.url()).toContain('/login');
      
      // Verify success message
      const message = await page.locator('.ant-message-success, .success-message').first();
      await expect(message).toBeVisible();
    }
  });

  test('T002: User Login - Valid Credentials', async ({ page }) => {
    // Navigate to login page
    await page.goto('/ui/login');
    
    // Enter valid credentials
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    
    // Click login button
    await page.click('button:has-text("Login"), button[type="submit"]');
    
    // Verify redirect to /ui
    await page.waitForURL('**/ui');
    expect(page.url()).toContain('/ui');
    
    // Verify user menu shows logged-in username
    const username = await page.locator('span:has-text("admin")').first();
    await expect(username).toBeVisible();
    
    // Verify JWT token is stored in localStorage
    const token = await page.evaluate(() => localStorage.getItem('token') || localStorage.getItem('auth_token'));
    expect(token).toBeTruthy();
  });

  test('T003: User Login - Invalid Credentials', async ({ page }) => {
    // Navigate to login page
    await page.goto('/ui/login');
    
    // Enter invalid credentials
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'invalid');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'wrongpassword');
    
    // Click login button
    await page.click('button:has-text("Login"), button[type="submit"]');
    
    // Verify error message is displayed
    const errorMessage = await page.locator('.ant-message-error, .error-message').first();
    await expect(errorMessage).toBeVisible();
    
    // Verify no redirect occurs
    expect(page.url()).toContain('/login');
    
    // Verify no token is stored
    const token = await page.evaluate(() => localStorage.getItem('token') || localStorage.getItem('auth_token'));
    expect(token).toBeNull();
  });

  test('T004: User Logout', async ({ page }) => {
    // Login first
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
    
    // Click logout button
    await page.click('button:has-text("Logout"), button:has-text("退出登录")');
    
    // Verify redirect to login
    await page.waitForURL('**/login');
    expect(page.url()).toContain('/login');
    
    // Verify JWT token is removed
    const token = await page.evaluate(() => localStorage.getItem('token') || localStorage.getItem('auth_token'));
    expect(token).toBeNull();
  });

  test('T005: Protected Route Access - Unauthorized', async ({ page }) => {
    // Clear localStorage (no auth token)
    await page.evaluate(() => {
      localStorage.clear();
    });
    
    // Try to access protected route
    await page.goto('/ui/configs');
    
    // Verify redirect to login
    await page.waitForURL('**/login');
    expect(page.url()).toContain('/login');
    
    // Try another protected route
    await page.goto('/ui/load-balancers');
    
    // Verify redirect to login
    expect(page.url()).toContain('/login');
  });

  test('T006: Forgot Password Flow', async ({ page }) => {
    // Navigate to forgot password page
    await page.goto('/ui/forgot-password');
    
    // Verify page title and form elements
    await expect(page.locator('h1, h2, h3').filter({ hasText: /forgot password|忘记密码/i })).toBeVisible();
    await expect(page.locator('input[type="email"]').first()).toBeVisible();
    
    // Enter invalid email format
    await page.fill('input[type="email"]', 'invalid-email');
    await page.click('button[type="submit"]');
    
    // Verify validation error
    const error = await page.locator('.ant-form-item-explain-error, .error-message').first();
    await expect(error).toBeVisible();
    
    // Enter valid email
    await page.fill('input[type="email"]', 'test@example.com');
    await page.click('button[type="submit"]');
    
    // Verify success message
    const success = await page.locator('.ant-message-success, .success-message').first();
    await expect(success).toBeVisible();
  });

  test('T007: Admin User Management - List Users', async ({ page }) => {
    // Login as admin
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
    
    // Navigate to users page
    await page.goto('/ui/users');
    
    // Verify user list table is displayed
    const table = await page.locator('table').first();
    await expect(table).toBeVisible();
    
    // Verify user columns
    await expect(page.locator('th:has-text("ID")').or(page.locator('th:has-text("ID")))).toBeVisible();
    await expect(page.locator('th:has-text("Username")').or(page.locator('th:has-text("用户名")))).toBeVisible();
    await expect(page.locator('th:has-text("Role")').or(page.locator('th:has-text("角色")))).toBeVisible();
    
    // Verify admin user is listed
    await expect(page.locator('td:has-text("admin")').first()).toBeVisible();
  });

  test('T008: Admin User Management - Create User', async ({ page }) => {
    // Login as admin
    await page.goto('/ui/login');
    await page.fill('input[placeholder*="username" i], input[type="text"]', 'admin');
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'admin123');
    await page.click('button:has-text("Login"), button[type="submit"]');
    await page.waitForURL('**/ui');
    
    // Navigate to users page
    await page.goto('/ui/users');
    
    // Click create user button
    await page.click('button:has-text("Create User"), button:has-text("创建用户")');
    
    // Fill in user details
    await page.fill('input[placeholder*="username" i], input[type="text"]', `testuser_${Date.now()}`);
    await page.fill('input[placeholder*="password" i], input[type="password"]', 'test123');
    
    // Select role if dropdown exists
    const roleSelect = page.locator('select, .ant-select').first();
    if (await roleSelect.count() > 0) {
      await roleSelect.click();
      await page.click('li:has-text("user")');
    }
    
    // Click save
    await page.click('button:has-text("Save"), button:has-text("保存")');
    
    // Verify success message
    const success = await page.locator('.ant-message-success, .success-message').first();
    await expect(success).toBeVisible();
    
    // Verify new user appears in list
    const username = `testuser_${Date.now()}`;
    await expect(page.locator(`td:has-text("${username}")`).first()).toBeVisible();
  });
});
