import React, { useEffect, useState } from 'react';
import { loadBalancerApi, HealthStatus } from '../services/loadBalancerApi';
import './LoadBalancerDetail.css';

interface HealthStatusPanelProps {
  loadBalancerId: string;
}

const HealthStatusPanel: React.FC<HealthStatusPanelProps> = ({ loadBalancerId }) => {
  const [healthData, setHealthData] = useState<{
    total_nodes: number;
    healthy_nodes: number;
    unhealthy_nodes: number;
    statuses: HealthStatus[];
  } | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchHealthStatus = async () => {
    try {
      setLoading(true);
      const data = await loadBalancerApi.getHealthStatus(loadBalancerId);
      setHealthData(data);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch health status');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchHealthStatus();
    // Refresh every 30 seconds
    const interval = setInterval(fetchHealthStatus, 30000);
    return () => clearInterval(interval);
  }, [loadBalancerId]);

  const handleTriggerCheck = async () => {
    try {
      await loadBalancerApi.triggerHealthCheck(loadBalancerId);
      // Refresh after triggering
      setTimeout(fetchHealthStatus, 2000);
    } catch (err: any) {
      setError(err.message || 'Failed to trigger health check');
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy':
        return '#10b981';
      case 'unhealthy':
        return '#ef4444';
      case 'unknown':
        return '#6b7280';
      default:
        return '#6b7280';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'healthy':
        return '✓';
      case 'unhealthy':
        return '✗';
      case 'unknown':
        return '?';
      default:
        return '?';
    }
  };

  if (loading && !healthData) {
    return <div className="panel">Loading health status...</div>;
  }

  if (error) {
    return (
      <div className="panel">
        <div className="error-message">Error: {error}</div>
        <button onClick={fetchHealthStatus}>Retry</button>
      </div>
    );
  }

  if (!healthData) {
    return null;
  }

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Health Status</h3>
        <button onClick={handleTriggerCheck} className="btn-secondary">
          Trigger Check
        </button>
      </div>

      <div className="health-summary">
        <div className="stat-card">
          <div className="stat-label">Total Nodes</div>
          <div className="stat-value">{healthData.total_nodes}</div>
        </div>
        <div className="stat-card" style={{ borderColor: '#10b981' }}>
          <div className="stat-label">Healthy</div>
          <div className="stat-value" style={{ color: '#10b981' }}>
            {healthData.healthy_nodes}
          </div>
        </div>
        <div className="stat-card" style={{ borderColor: '#ef4444' }}>
          <div className="stat-label">Unhealthy</div>
          <div className="stat-value" style={{ color: '#ef4444' }}>
            {healthData.unhealthy_nodes}
          </div>
        </div>
      </div>

      <div className="health-status-list">
        {healthData.statuses.map((status) => (
          <div key={status.config_id} className="health-status-item">
            <div className="status-header">
              <span
                className="status-icon"
                style={{ backgroundColor: getStatusColor(status.status) }}
              >
                {getStatusIcon(status.status)}
              </span>
              <span className="config-id">{status.config_id}</span>
              <span className="status-badge" style={{ color: getStatusColor(status.status) }}>
                {status.status.toUpperCase()}
              </span>
            </div>
            <div className="status-details">
              <div className="detail-item">
                <span className="detail-label">Response Time:</span>
                <span className="detail-value">{status.response_time_ms}ms</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Consecutive Successes:</span>
                <span className="detail-value">{status.consecutive_successes}</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Consecutive Failures:</span>
                <span className="detail-value">{status.consecutive_failures}</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Last Check:</span>
                <span className="detail-value">
                  {new Date(status.last_check_time).toLocaleString()}
                </span>
              </div>
              {status.last_error && (
                <div className="detail-item error">
                  <span className="detail-label">Last Error:</span>
                  <span className="detail-value">{status.last_error}</span>
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default HealthStatusPanel;
