package database

import (
	"testing"
	"time"
)

func newTestAlert(loadBalancerID, level, alertType string) *Alert {
	return &Alert{
		LoadBalancerID: loadBalancerID,
		Level:          level,
		Type:           alertType,
		Message:        "test message for " + alertType,
		Details:        "test details",
	}
}

func TestCreateAndGetAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	a := newTestAlert("lb-1", "critical", "all_nodes_down")
	if err := CreateAlert(a); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected CreateAlert to assign an ID")
	}

	got, err := GetAlert(a.ID)
	if err != nil {
		t.Fatalf("GetAlert: %v", err)
	}
	if got.LoadBalancerID != "lb-1" || got.Level != "critical" || got.Type != "all_nodes_down" {
		t.Errorf("unexpected alert: %+v", got)
	}
	if got.Acknowledged {
		t.Error("new alert should not be acknowledged")
	}
	if got.AcknowledgedAt != nil {
		t.Error("new alert should have nil AcknowledgedAt")
	}
}

func TestCreateAlert_PreservesGivenID(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	a := newTestAlert("lb-1", "info", "custom")
	a.ID = "fixed-alert-id"
	if err := CreateAlert(a); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if a.ID != "fixed-alert-id" {
		t.Errorf("CreateAlert should not overwrite an explicit ID, got %q", a.ID)
	}
}

func TestGetAlert_NotFound(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	_, err := GetAlert("does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing alert")
	}
}

func TestGetAlertsByLoadBalancer_FilteringAndLimit(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	for i := 0; i < 3; i++ {
		a := newTestAlert("lb-filter", "warning", "type_a")
		if err := CreateAlert(a); err != nil {
			t.Fatalf("CreateAlert: %v", err)
		}
		if i == 0 {
			if err := AcknowledgeAlert(a.ID); err != nil {
				t.Fatalf("AcknowledgeAlert: %v", err)
			}
		}
	}
	// alert for a different load balancer must not leak in
	other := newTestAlert("lb-other", "warning", "type_a")
	if err := CreateAlert(other); err != nil {
		t.Fatalf("CreateAlert other: %v", err)
	}

	all, err := GetAlertsByLoadBalancer("lb-filter", nil, 0)
	if err != nil {
		t.Fatalf("GetAlertsByLoadBalancer: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 alerts for lb-filter, got %d", len(all))
	}

	unacked := false
	unackedAlerts, err := GetAlertsByLoadBalancer("lb-filter", &unacked, 0)
	if err != nil {
		t.Fatalf("GetAlertsByLoadBalancer unacked: %v", err)
	}
	if len(unackedAlerts) != 2 {
		t.Fatalf("expected 2 unacknowledged alerts, got %d", len(unackedAlerts))
	}

	acked := true
	ackedAlerts, err := GetAlertsByLoadBalancer("lb-filter", &acked, 0)
	if err != nil {
		t.Fatalf("GetAlertsByLoadBalancer acked: %v", err)
	}
	if len(ackedAlerts) != 1 {
		t.Fatalf("expected 1 acknowledged alert, got %d", len(ackedAlerts))
	}
	if ackedAlerts[0].AcknowledgedAt == nil {
		t.Error("acknowledged alert should have AcknowledgedAt set")
	}

	limited, err := GetAlertsByLoadBalancer("lb-filter", nil, 2)
	if err != nil {
		t.Fatalf("GetAlertsByLoadBalancer limited: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected limit=2 to yield 2 rows, got %d", len(limited))
	}
}

func TestGetAllAlerts_FilteringAndLimit(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	if err := CreateAlert(newTestAlert("lb-a", "critical", "t1")); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if err := CreateAlert(newTestAlert("lb-b", "warning", "t2")); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if err := CreateAlert(newTestAlert("lb-c", "warning", "t3")); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}

	all, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 alerts total, got %d", len(all))
	}

	warnings, err := GetAllAlerts(nil, "warning", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts level filter: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warning alerts, got %d", len(warnings))
	}

	unacked := false
	unackedWarnings, err := GetAllAlerts(&unacked, "warning", 1)
	if err != nil {
		t.Fatalf("GetAllAlerts combined filter: %v", err)
	}
	if len(unackedWarnings) != 1 {
		t.Fatalf("expected limit=1 to yield 1 row, got %d", len(unackedWarnings))
	}
}

func TestAcknowledgeAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	a := newTestAlert("lb-ack", "info", "t")
	if err := CreateAlert(a); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if err := AcknowledgeAlert(a.ID); err != nil {
		t.Fatalf("AcknowledgeAlert: %v", err)
	}
	got, err := GetAlert(a.ID)
	if err != nil {
		t.Fatalf("GetAlert: %v", err)
	}
	if !got.Acknowledged || got.AcknowledgedAt == nil {
		t.Errorf("expected alert to be acknowledged with timestamp, got %+v", got)
	}

	// Acknowledging a non-existent alert is a no-op UPDATE and must not error.
	if err := AcknowledgeAlert("does-not-exist"); err != nil {
		t.Errorf("AcknowledgeAlert on missing id should not error, got %v", err)
	}
}

func TestDeleteAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	a := newTestAlert("lb-del", "info", "t")
	if err := CreateAlert(a); err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if err := DeleteAlert(a.ID); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}
	if _, err := GetAlert(a.ID); err == nil {
		t.Fatal("expected alert to be gone after delete")
	}

	// Deleting a non-existent alert must not error.
	if err := DeleteAlert("does-not-exist"); err != nil {
		t.Errorf("DeleteAlert on missing id should not error, got %v", err)
	}
}

func TestDeleteOldAlerts(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	recent := newTestAlert("lb-age", "info", "recent")
	if err := CreateAlert(recent); err != nil {
		t.Fatalf("CreateAlert recent: %v", err)
	}
	old := newTestAlert("lb-age", "info", "old")
	if err := CreateAlert(old); err != nil {
		t.Fatalf("CreateAlert old: %v", err)
	}
	backdate := time.Now().UTC().Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := DB.Exec("UPDATE alerts SET created_at = ? WHERE id = ?", backdate, old.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := DeleteOldAlerts(7); err != nil {
		t.Fatalf("DeleteOldAlerts: %v", err)
	}

	if _, err := GetAlert(old.ID); err == nil {
		t.Error("expected old alert to be deleted")
	}
	if _, err := GetAlert(recent.ID); err != nil {
		t.Errorf("expected recent alert to survive, got err: %v", err)
	}
}

func TestCountUnacknowledgedAlerts(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")

	a1 := newTestAlert("lb-count", "info", "t1")
	a2 := newTestAlert("lb-count", "info", "t2")
	a3 := newTestAlert("lb-count", "info", "t3")
	for _, a := range []*Alert{a1, a2, a3} {
		if err := CreateAlert(a); err != nil {
			t.Fatalf("CreateAlert: %v", err)
		}
	}
	if err := AcknowledgeAlert(a1.ID); err != nil {
		t.Fatalf("AcknowledgeAlert: %v", err)
	}

	count, err := CountUnacknowledgedAlerts("lb-count")
	if err != nil {
		t.Fatalf("CountUnacknowledgedAlerts: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unacknowledged alerts, got %d", count)
	}

	zero, err := CountUnacknowledgedAlerts("lb-does-not-exist")
	if err != nil {
		t.Fatalf("CountUnacknowledgedAlerts unknown lb: %v", err)
	}
	if zero != 0 {
		t.Errorf("expected 0 for unknown load balancer, got %d", zero)
	}
}

// setupLBWithNodes creates a load balancer with the given config node IDs
// (all enabled) for exercising the Check*Alert helpers.
func setupLBWithNodes(t *testing.T, id string, configIDs ...string) *LoadBalancer {
	t.Helper()
	nodes := make([]ConfigNode, len(configIDs))
	for i, cid := range configIDs {
		nodes[i] = ConfigNode{ConfigID: cid, Weight: 1, Enabled: true}
	}
	lb := &LoadBalancer{
		ID:          id,
		Name:        "lb-" + id,
		Strategy:    "round_robin",
		ConfigNodes: nodes,
		Enabled:     true,
	}
	if err := CreateLoadBalancer(lb); err != nil {
		t.Fatalf("CreateLoadBalancer: %v", err)
	}
	return lb
}

func TestCheckAndCreateAllNodesDownAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM health_statuses")
	DB.Exec("DELETE FROM load_balancers")

	lb := setupLBWithNodes(t, "lb-down-1", "node-1", "node-2")

	// No health status rows at all => treated as all down (GetHealthStatus errors, allDown stays true).
	if err := CheckAndCreateAllNodesDownAlert(lb.ID); err != nil {
		t.Fatalf("CheckAndCreateAllNodesDownAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	found := false
	for _, a := range alerts {
		if a.Type == "all_nodes_down" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected an all_nodes_down alert to be created")
	}

	// Calling again must not create a duplicate (dedup against existing unacknowledged alert).
	if err := CheckAndCreateAllNodesDownAlert(lb.ID); err != nil {
		t.Fatalf("CheckAndCreateAllNodesDownAlert second call: %v", err)
	}
	alerts, err = GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	count := 0
	for _, a := range alerts {
		if a.Type == "all_nodes_down" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 all_nodes_down alert after dedup, got %d", count)
	}

	// Mark one node healthy: should not create a new alert, existing behavior unaffected.
	DB.Exec("DELETE FROM alerts")
	if err := CreateOrUpdateHealthStatus(&HealthStatus{ConfigID: "node-1", Status: "healthy", LastCheckTime: time.Now()}); err != nil {
		t.Fatalf("CreateOrUpdateHealthStatus: %v", err)
	}
	if err := CheckAndCreateAllNodesDownAlert(lb.ID); err != nil {
		t.Fatalf("CheckAndCreateAllNodesDownAlert with healthy node: %v", err)
	}
	alerts, err = GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	for _, a := range alerts {
		if a.Type == "all_nodes_down" {
			t.Error("did not expect all_nodes_down alert when a node is healthy")
		}
	}
}

func TestCheckAndCreateAllNodesDownAlert_UnknownLoadBalancer(t *testing.T) {
	ensureTestDB(t)
	if err := CheckAndCreateAllNodesDownAlert("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown load balancer")
	}
}

func TestCheckAndCreateAllNodesDownAlert_DisabledNodesIgnored(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM health_statuses")
	DB.Exec("DELETE FROM load_balancers")

	lb := &LoadBalancer{
		ID:       "lb-disabled",
		Name:     "lb-disabled",
		Strategy: "round_robin",
		ConfigNodes: []ConfigNode{
			{ConfigID: "node-disabled", Weight: 1, Enabled: false},
		},
		Enabled: true,
	}
	if err := CreateLoadBalancer(lb); err != nil {
		t.Fatalf("CreateLoadBalancer: %v", err)
	}
	// All nodes disabled => loop never flips allDown to false, alert still fires.
	if err := CheckAndCreateAllNodesDownAlert(lb.ID); err != nil {
		t.Fatalf("CheckAndCreateAllNodesDownAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Type != "all_nodes_down" {
		t.Errorf("expected all_nodes_down alert even with only disabled nodes, got %+v", alerts)
	}
}

func TestCheckAndCreateLowHealthyNodesAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM health_statuses")
	DB.Exec("DELETE FROM load_balancers")

	lb := setupLBWithNodes(t, "lb-low-1", "node-a", "node-b", "node-c")
	for _, id := range []string{"node-a"} {
		if err := CreateOrUpdateHealthStatus(&HealthStatus{ConfigID: id, Status: "healthy", LastCheckTime: time.Now()}); err != nil {
			t.Fatalf("CreateOrUpdateHealthStatus: %v", err)
		}
	}

	// Only 1 healthy out of 3, threshold 2 => alert created.
	if err := CheckAndCreateLowHealthyNodesAlert(lb.ID, 2); err != nil {
		t.Fatalf("CheckAndCreateLowHealthyNodesAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	found := false
	for _, a := range alerts {
		if a.Type == "low_healthy_nodes" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected low_healthy_nodes alert")
	}

	// Dedup: calling again must not add a second alert of the same type.
	if err := CheckAndCreateLowHealthyNodesAlert(lb.ID, 2); err != nil {
		t.Fatalf("CheckAndCreateLowHealthyNodesAlert second call: %v", err)
	}
	alerts, err = GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	count := 0
	for _, a := range alerts {
		if a.Type == "low_healthy_nodes" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 low_healthy_nodes alert after dedup, got %d", count)
	}
}

func TestCheckAndCreateLowHealthyNodesAlert_ThresholdMet_NoAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM health_statuses")
	DB.Exec("DELETE FROM load_balancers")

	lb := setupLBWithNodes(t, "lb-low-2", "node-x", "node-y")
	for _, id := range []string{"node-x", "node-y"} {
		if err := CreateOrUpdateHealthStatus(&HealthStatus{ConfigID: id, Status: "healthy", LastCheckTime: time.Now()}); err != nil {
			t.Fatalf("CreateOrUpdateHealthStatus: %v", err)
		}
	}

	if err := CheckAndCreateLowHealthyNodesAlert(lb.ID, 2); err != nil {
		t.Fatalf("CheckAndCreateLowHealthyNodesAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	for _, a := range alerts {
		if a.Type == "low_healthy_nodes" {
			t.Error("did not expect low_healthy_nodes alert when threshold is met")
		}
	}
}

func TestCheckAndCreateLowHealthyNodesAlert_UnknownLoadBalancer(t *testing.T) {
	ensureTestDB(t)
	if err := CheckAndCreateLowHealthyNodesAlert("does-not-exist", 1); err == nil {
		t.Fatal("expected error for unknown load balancer")
	}
}

func insertLBRequestLogAt(t *testing.T, id, lbID, configID string, success bool, when time.Time) {
	t.Helper()
	_, err := DB.Exec(`
		INSERT INTO load_balancer_request_logs (
			id, load_balancer_id, selected_config_id, request_time, response_time,
			duration_ms, status_code, success, retry_count, created_at
		) VALUES (?, ?, ?, ?, ?, 10, 200, ?, 0, ?)
	`, id, lbID, configID, when.UTC().Format("2006-01-02 15:04:05"), when.UTC().Format("2006-01-02 15:04:05"), success, when.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("insertLBRequestLogAt: %v", err)
	}
}

func TestCheckAndCreateHighErrorRateAlert(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM load_balancer_request_logs")
	DB.Exec("DELETE FROM load_balancers")

	lb := setupLBWithNodes(t, "lb-err-1", "node-1")
	now := time.Now()
	// 2 successes, 3 failures => 60% error rate, above a 50% threshold.
	insertLBRequestLogAt(t, "req-1", lb.ID, "node-1", true, now)
	insertLBRequestLogAt(t, "req-2", lb.ID, "node-1", true, now)
	insertLBRequestLogAt(t, "req-3", lb.ID, "node-1", false, now)
	insertLBRequestLogAt(t, "req-4", lb.ID, "node-1", false, now)
	insertLBRequestLogAt(t, "req-5", lb.ID, "node-1", false, now)

	if err := CheckAndCreateHighErrorRateAlert(lb.ID, 0.5, 10); err != nil {
		t.Fatalf("CheckAndCreateHighErrorRateAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	found := false
	for _, a := range alerts {
		if a.Type == "high_error_rate" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected high_error_rate alert")
	}

	// Dedup on repeat call.
	if err := CheckAndCreateHighErrorRateAlert(lb.ID, 0.5, 10); err != nil {
		t.Fatalf("CheckAndCreateHighErrorRateAlert second call: %v", err)
	}
	alerts, err = GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	count := 0
	for _, a := range alerts {
		if a.Type == "high_error_rate" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 high_error_rate alert after dedup, got %d", count)
	}
}

func TestCheckAndCreateHighErrorRateAlert_NoRequests(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM load_balancer_request_logs")

	if err := CheckAndCreateHighErrorRateAlert("lb-with-no-requests", 0.5, 10); err != nil {
		t.Fatalf("CheckAndCreateHighErrorRateAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected no alerts with zero requests in window, got %d", len(alerts))
	}
}

func TestCheckAndCreateHighErrorRateAlert_BelowThreshold(t *testing.T) {
	ensureTestDB(t)
	DB.Exec("DELETE FROM alerts")
	DB.Exec("DELETE FROM load_balancer_request_logs")
	DB.Exec("DELETE FROM load_balancers")

	lb := setupLBWithNodes(t, "lb-err-2", "node-1")
	now := time.Now()
	insertLBRequestLogAt(t, "req-ok-1", lb.ID, "node-1", true, now)
	insertLBRequestLogAt(t, "req-ok-2", lb.ID, "node-1", true, now)
	insertLBRequestLogAt(t, "req-ok-3", lb.ID, "node-1", false, now)

	if err := CheckAndCreateHighErrorRateAlert(lb.ID, 0.5, 10); err != nil {
		t.Fatalf("CheckAndCreateHighErrorRateAlert: %v", err)
	}
	alerts, err := GetAllAlerts(nil, "", 0)
	if err != nil {
		t.Fatalf("GetAllAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected no alert when error rate is below threshold, got %d", len(alerts))
	}
}

func TestBoolPtr(t *testing.T) {
	p := boolPtr(true)
	if p == nil || *p != true {
		t.Fatal("boolPtr(true) should return pointer to true")
	}
	p2 := boolPtr(false)
	if p2 == nil || *p2 != false {
		t.Fatal("boolPtr(false) should return pointer to false")
	}
}
