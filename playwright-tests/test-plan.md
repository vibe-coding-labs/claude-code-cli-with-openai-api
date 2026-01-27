# Playwright Test Plan - Claude-to-OpenAI API Proxy

## Overview
This document outlines 30+ comprehensive test cases for the Claude-to-OpenAI API Proxy product using Playwright MCP.

**Test Environment:**
- Frontend URL: http://localhost:54990
- Backend URL: http://localhost:8083
- Test User: admin / admin123 (default)

---

## Category 1: Authentication & User Management (8 tests)

### T001: User Initialization
**Description:** Verify first-time user initialization flow
**Steps:**
1. Navigate to http://localhost:54990
2. Verify redirect to /ui/initialize
3. Fill in username (admin)
4. Fill in password (admin123)
5. Click "Initialize" button
6. Verify redirect to /ui/login
7. Verify initialization success message

### T002: User Login - Valid Credentials
**Description:** Verify successful login with valid credentials
**Steps:**
1. Navigate to http://localhost:54990/ui/login
2. Enter valid username (admin)
3. Enter valid password (admin123)
4. Click "Login" button
5. Verify redirect to /ui
6. Verify user menu shows logged-in username
7. Verify JWT token is stored in localStorage

### T003: User Login - Invalid Credentials
**Description:** Verify login failure with invalid credentials
**Steps:**
1. Navigate to http://localhost:54990/ui/login
2. Enter invalid username or password
3. Click "Login" button
4. Verify error message is displayed
5. Verify no redirect occurs
6. Verify no token is stored

### T004: User Logout
**Description:** Verify logout functionality
**Steps:**
1. Login with valid credentials
2. Click "Logout" button in header
3. Verify redirect to /ui/login
4. Verify JWT token is removed from localStorage
5. Verify accessing protected routes redirects to login

### T005: Protected Route Access - Unauthorized
**Description:** Verify protected routes redirect to login when not authenticated
**Steps:**
1. Clear localStorage (no auth token)
2. Navigate to http://localhost:54990/ui/configs
3. Verify redirect to /ui/login
4. Navigate to http://localhost:54990/ui/load-balancers
5. Verify redirect to /ui/login

### T006: Forgot Password Flow
**Description:** Verify forgot password page loads and validates input
**Steps:**
1. Navigate to http://localhost:54990/ui/forgot-password
2. Verify page title and form elements
3. Enter invalid email format
4. Verify validation error
5. Enter valid email
6. Verify submission shows success message

### T007: Admin User Management - List Users
**Description:** Verify admin can view user list
**Steps:**
1. Login as admin
2. Navigate to /ui/users
3. Verify user list table is displayed
4. Verify user columns: ID, Username, Role, Status, Created At
5. Verify admin user is listed

### T008: Admin User Management - Create User
**Description:** Verify admin can create new users
**Steps:**
1. Login as admin
2. Navigate to /ui/users
3. Click "Create User" button
4. Fill in username, password, role
5. Click "Save"
6. Verify success message
7. Verify new user appears in list

---

## Category 2: API Configuration Management (10 tests)

### T009: API Config List View
**Description:** Verify API configuration list displays correctly
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Verify config list table is displayed
4. Verify columns: Name, API Key, Base URL, Models, Status, Actions
4. Verify "Create Config" button is visible

### T010: Create API Configuration - Basic
**Description:** Verify creating a new API configuration
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Create Config" button
4. Fill in name: "Test Config 1"
5. Enter OpenAI API Key
6. Enter Base URL: https://api.openai.com/v1
7. Select models: gpt-4o, gpt-4o-mini
8. Click "Save"
9. Verify success message
10. Verify config appears in list

### T011: Create API Configuration - Validation
**Description:** Verify form validation for creating configs
**Steps:**
1. Navigate to /ui/configs/create
2. Click "Save" without filling required fields
3. Verify validation errors for required fields
4. Enter invalid URL format
5. Verify URL validation error
6. Verify form cannot be submitted with invalid data

### T012: Edit API Configuration
**Description:** Verify editing an existing configuration
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Edit" on an existing config
4. Modify name or settings
5. Click "Save"
6. Verify success message
7. Verify changes are reflected in list

### T013: Delete API Configuration
**Description:** Verify deleting a configuration
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Delete" on a config
4. Confirm deletion in modal
5. Verify success message
6. Verify config is removed from list

### T014: Renew API Key
**Description:** Verify renewing API key functionality
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Renew Key" on a config
4. Confirm action in modal
5. Verify new API key is generated
6. Verify old key is invalidated

### T015: Enable/Disable Configuration
**Description:** Verify enabling and disabling configurations
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Disable" on an enabled config
4. Verify status changes to disabled
5. Click "Enable" on the config
6. Verify status changes to enabled

### T016: View Configuration Details
**Description:** Viewing detailed configuration information
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click on a config name to view details
4. Verify all config fields are displayed
5. Verify Anthropic API key is shown
6. Verify usage statistics are visible

### T017: Test API Configuration - Basic
**Description:** Verify testing an API configuration
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Test" on a config
4. Enter test message: "Hello, world!"
5. Select model: gpt-4o
6. Set max_tokens: 100
7. Click "Send Test"
8. Verify response is displayed
9. Verify token statistics

### T018: Test API Configuration - Streaming
**Description:** Verify streaming response in test
**Steps:**
1. Navigate to config test page
2. Enable "Stream" option
3. Send test request
4. Verify streaming response is displayed in real-time
5. Verify complete response is shown

---

## Category 3: Load Balancer Management (8 tests)

### T019: Load Balancer List View
**Description:** Verify load balancer list displays correctly
**Steps:**
1. Login as admin
2. Navigate to /ui/load-balancers
3. Verify load balancer list is displayed
4. Verify columns: Name, Strategy, Nodes, Status, Actions
5. Verify "Create Load Balancer" button

### T020: Create Load Balancer - Round Robin
**Description:** Verify creating load balancer with round-robin strategy
**Steps:**
1. Login as admin
2. Navigate to /ui/load-balancers
3. Click "Create Load Balancer"
4. Enter name: "Test LB 1"
5. Select strategy: "Round Robin"
6. Add config nodes (at least 2)
7. Configure health check settings
8. Click "Save"
9. Verify success message
10. Verify load balancer appears in list

### T021: Create Load Balancer - Weighted
**Description:** Verify creating load balancer with weighted strategy
**Steps:**
1. Navigate to /ui/load-balancers/create
2. Enter name: "Test LB 2"
3. Select strategy: "Weighted Round Robin"
4. Add config nodes with different weights
5. Verify weight inputs work correctly
6. Click "Save"
7. Verify load balancer is created with weights

### T022: Edit Load Balancer Configuration
**Description:** Verify editing load balancer settings
**Steps:**
1. Login as admin
2. Navigate to /ui/load-balancers
3. Click "Edit" on a load balancer
4. Modify strategy or health check settings
5. Click "Save"
6. Verify success message
7. Verify changes are applied

### T023: View Load Balancer Details
**Description:** Viewing detailed load balancer information
**Steps:**
1. Login as admin
2. Navigate to /ui/load-balancers
3. Click on a load balancer name
4. Verify overview section is displayed
5. Verify health status panel shows node health
6. Verify circuit breaker status is visible
7. Verify statistics charts are displayed

### T024: Load Balancer Health Check Monitoring
**Description:** Verify real-time health check status updates
**Steps:**
1. Navigate to load balancer details
2. Observe health status panel
3. Verify node health indicators (Healthy/Unhealthy)
4. Verify health check timestamps
5. Verify status updates in real-time

### T025: Load Balancer Circuit Breaker Status
**Description:** Verify circuit breaker status display
**Steps:**
1. Navigate to load balancer details
2. View circuit breaker panel
3. Verify state is displayed (Closed/Open/Half-Open)
4. Verify error rate is shown
5. Verify circuit breaker metrics

### T026: Delete Load Balancer
**Description:** Verify deleting a load balancer
**Steps:**
1. Login as admin
2. Navigate to /ui/load-balancers
3. Click "Delete" on a load balancer
4. Confirm deletion in modal
5. Verify success message
6. Verify load balancer is removed from list

---

## Category 4: Monitoring & Analytics (4 tests)

### T027: View Request Logs
**Description:** Verify request logs display correctly
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Logs" tab
4. Verify logs table is displayed
5. Verify columns: Timestamp, Method, Path, Status, Duration
6. Click on a log entry to view details
7. Verify full request/response JSON is shown

### T028: Filter Request Logs
**Description:** Verify log filtering functionality
**Steps:**
1. Navigate to request logs page
2. Apply date range filter
3. Apply status code filter (e.g., 200, 500)
4. Verify filtered results
5. Clear filters
6. Verify all logs are displayed

### T029: View Statistics Dashboard
**Description:** Verify statistics display correctly
**Steps:**
1. Login as admin
2. Navigate to /ui
3. Click "Statistics" tab
4. Verify request count is displayed
5. Verify success rate percentage
6. Verify token usage statistics
7. Verify response time metrics (P50, P95, P99)

### T030: View Alerts Panel
**Description:** Verify alerts are displayed correctly
**Steps:**
1. Login as admin
2. Navigate to load balancer details
3. Click "Alerts" tab
4. Verify alerts list is displayed
5. Verify alert severity levels (Critical/Warning/Info)
6. Click "Acknowledge" on an alert
7. Verify alert is marked as acknowledged

---

## Category 5: Integration & Edge Cases (4 tests)

### T031: Concurrent Request Handling
**Description:** Verify system handles multiple concurrent requests
**Steps:**
1. Login as admin
2. Open multiple browser tabs
3. Send test requests simultaneously from each tab
4. Verify all requests complete successfully
5. Verify no errors or timeouts occur

### T032: API Error Handling
**Description:** Verify proper error handling for API failures
**Steps:**
1. Create config with invalid API key
2. Attempt to test configuration
3. Verify error message is displayed
4. Verify error is logged
5. Verify system remains stable

### T033: Session Timeout
**Description:** Verify session timeout functionality
**Steps:**
1. Login as admin
2. Wait for session to expire (if configured)
3. Attempt to navigate to protected route
4. Verify redirect to login page
5. Verify appropriate timeout message

### T034: Responsive Design
**Description:** Verify UI is responsive across screen sizes
**Steps:**
1. Login as admin
2. Resize browser window to mobile size (< 768px)
3. Verify sidebar collapses or adapts
4. Verify tables have horizontal scroll
5. Verify buttons and controls remain accessible
6. Resize to tablet size (768px - 1024px)
7. Verify layout adapts correctly

---

## Test Execution Strategy

### Prerequisites
1. Backend service running on http://localhost:8083
2. Frontend service running on http://localhost:54990
3. Database initialized with test data
4. Admin user created (admin/admin123)

### Test Execution Order
1. Run Category 1 (Authentication) first
2. Run Category 2 (API Config) second
3. Run Category 3 (Load Balancers) third
4. Run Category 4 (Monitoring) fourth
5. Run Category 5 (Integration) last

### Expected Results
- All tests should pass with valid test data
- Error messages should be clear and actionable
- UI should be responsive and accessible
- Data should persist correctly across operations

---

## Test Data Requirements

### Test API Configuration
- Name: Test Config 1
- API Key: sk-test-xxxxxxxxxxxx
- Base URL: https://api.openai.com/v1
- Models: gpt-4o, gpt-4o-mini

### Test Load Balancer
- Name: Test LB 1
- Strategy: Round Robin
- Nodes: 2+ API configurations

### Test User (non-admin)
- Username: testuser
- Password: test123
- Role: user

---

## Notes
- Tests should be run in isolation where possible
- Clean up test data after each test suite
- Use page screenshots for debugging failed tests
- Log network requests and responses for API tests
- Verify both happy path and error scenarios
