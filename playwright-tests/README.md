# Playwright E2E Tests

This directory contains comprehensive end-to-end tests for the Claude-to-OpenAI API Proxy using Playwright.

## Test Coverage

The test suite includes **34 test cases** organized into 5 categories:

### 1. Authentication & User Management (8 tests)
- T001: User Initialization
- T002: User Login - Valid Credentials
- T003: User Login - Invalid Credentials
- T004: User Logout
- T005: Protected Route Access - Unauthorized
- T006: Forgot Password Flow
- T007: Admin User Management - List Users
- T008: Admin User Management - Create User

### 2. API Configuration Management (10 tests)
- T009: API Config List View
- T010: Create API Configuration - Basic
- T011: Create API Configuration - Validation
- T012: Edit API Configuration
- T013: Delete API Configuration
- T014: Renew API Key
- T015: Enable/Disable Configuration
- T016: View Configuration Details
- T017: Test API Configuration - Basic
- T018: Test API Configuration - Streaming

### 3. Load Balancer Management (8 tests)
- T019: Load Balancer List View
- T020: Create Load Balancer - Round Robin
- T021: Create Load Balancer - Weighted
- T022: Edit Load Balancer Configuration
- T023: View Load Balancer Details
- T024: Load Balancer Health Check Monitoring
- T025: Load Balancer Circuit Breaker Status
- T026: Delete Load Balancer

### 4. Monitoring & Analytics (4 tests)
- T027: View Request Logs
- T028: Filter Request Logs
- T029: View Statistics Dashboard
- T030: View Alerts Panel

### 5. Integration & Edge Cases (4 tests)
- T031: Concurrent Request Handling
- T032: API Error Handling
- T033: Session Timeout
- T034: Responsive Design

## Prerequisites

1. **Backend Service**: Ensure the backend is running on `http://localhost:8083`
2. **Frontend Service**: Ensure the frontend is running on `http://localhost:54990`
3. **Test User**: Default admin credentials are `admin` / `admin123`

## Installation

```bash
cd playwright-tests
npm install
npx playwright install chromium
```

## Running Tests

### Run all tests
```bash
npm test
```

### Run tests in headed mode (watch browser)
```bash
npm run test:headed
```

### Run tests with UI mode
```bash
npm run test:ui
```

### Debug tests
```bash
npm run test:debug
```

### Run specific test file
```bash
npx playwright test tests/auth/auth.spec.ts
```

### Run specific test
```bash
npx playwright test -g "T001: User Initialization"
```

## Test Reports

After running tests, view the HTML report:
```bash
npx playwright show-report
```

## Configuration

The Playwright configuration is in `playwright.config.ts`:

- **Base URL**: `http://localhost:54990`
- **Browser**: Chromium
- **Parallel Execution**: Enabled
- **Screenshots**: On failure only
- **Video**: Retained on failure
- **Trace**: On first retry

## Test Data

### Default Credentials
- Username: `admin`
- Password: `admin123`

### Test API Configuration
- Name: `Test Config 1`
- API Key: `sk-test-xxxxxxxxxxxx`
- Base URL: `https://api.openai.com/v1`
- Models: `gpt-4o`, `gpt-4o-mini`

### Test Load Balancer
- Name: `Test LB 1`
- Strategy: `Round Robin`
- Nodes: 2+ API configurations

## CI/CD Integration

For CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Install dependencies
  run: npm install
  
- name: Install Playwright browsers
  run: npx playwright install --with-deps chromium
  
- name: Run Playwright tests
  run: npm test
  
- name: Upload test results
  if: always()
  uses: actions/upload-artifact@v3
  with:
    name: playwright-report
    path: playwright-report/
```

## Troubleshooting

### Tests fail with "Service not available"
- Ensure both frontend and backend services are running
- Check that ports 54990 and 8083 are not blocked

### Tests timeout
- Increase timeout in `playwright.config.ts`
- Check if the services are responding slowly

### Element not found errors
- The UI may have changed - update selectors accordingly
- Use `npm run test:debug` to inspect the page state

## Contributing

When adding new tests:

1. Follow the existing test structure
2. Use descriptive test names following the `T###: Description` pattern
3. Group tests logically in appropriate describe blocks
4. Add test documentation to this README
5. Ensure tests are independent and can run in parallel

## Resources

- [Playwright Documentation](https://playwright.dev)
- [Playwright Best Practices](https://playwright.dev/docs/best-practices)
- [Test Plan](./test-plan.md) - Detailed test case documentation
