/* eslint-disable react-hooks/exhaustive-deps */
import React, { useEffect, useState } from 'react';
import { loadBalancerApi, Alert } from '../services/loadBalancerApi';
import './LoadBalancerDetail.css';

interface AlertsPanelProps {
  loadBalancerId: string;
}

const AlertsPanel: React.FC<AlertsPanelProps> = ({ loadBalancerId }) => {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<'all' | 'active' | 'acknowledged'>('active');

  const fetchAlerts = async () => {
    try {
      setLoading(true);
      const data = await loadBalancerApi.getAlerts(loadBalancerId);
      setAlerts(data.alerts || []);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch alerts');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAlerts();
    // Refresh every 30 seconds
    const interval = setInterval(fetchAlerts, 30000);
    return () => clearInterval(interval);
  }, [loadBalancerId]);

  const handleAcknowledge = async (alertId: string) => {
    try {
      await loadBalancerApi.acknowledgeAlert(loadBalancerId, alertId);
      // Refresh after acknowledging
      setTimeout(fetchAlerts, 500);
    } catch (err: any) {
      setError(err.message || 'Failed to acknowledge alert');
    }
  };

  const getSeverityColor = (level: string) => {
    switch (level) {
      case 'critical':
        return '#dc2626';
      case 'warning':
        return '#f59e0b';
      case 'info':
        return '#3b82f6';
      default:
        return '#6b7280';
    }
  };

  const getSeverityIcon = (level: string) => {
    switch (level) {
      case 'critical':
        return '⚠';
      case 'warning':
        return '⚡';
      case 'info':
        return 'ℹ';
      default:
        return '•';
    }
  };

  const filteredAlerts = alerts.filter((alert) => {
    if (filter === 'active') return !alert.acknowledged;
    if (filter === 'acknowledged') return alert.acknowledged;
    return true;
  });

  if (loading && alerts.length === 0) {
    return <div className="panel">Loading alerts...</div>;
  }

  if (error) {
    return (
      <div className="panel">
        <div className="error-message">Error: {error}</div>
        <button onClick={fetchAlerts}>Retry</button>
      </div>
    );
  }

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Alerts</h3>
        <div className="filter-buttons">
          <button
            className={filter === 'active' ? 'btn-filter-active' : 'btn-filter'}
            onClick={() => setFilter('active')}
          >
            Active ({alerts.filter((a) => !a.acknowledged).length})
          </button>
          <button
            className={filter === 'acknowledged' ? 'btn-filter-active' : 'btn-filter'}
            onClick={() => setFilter('acknowledged')}
          >
            Acknowledged ({alerts.filter((a) => a.acknowledged).length})
          </button>
          <button
            className={filter === 'all' ? 'btn-filter-active' : 'btn-filter'}
            onClick={() => setFilter('all')}
          >
            All ({alerts.length})
          </button>
        </div>
      </div>

      {filteredAlerts.length === 0 ? (
        <div className="empty-state">
          {filter === 'active' ? 'No active alerts' : 'No alerts found'}
        </div>
      ) : (
        <div className="alerts-list">
          {filteredAlerts.map((alert) => (
            <div
              key={alert.id}
              className={`alert-item ${alert.acknowledged ? 'acknowledged' : ''}`}
              style={{ borderLeftColor: getSeverityColor(alert.level) }}
            >
              <div className="alert-header">
                <span
                  className="severity-icon"
                  style={{ color: getSeverityColor(alert.level) }}
                >
                  {getSeverityIcon(alert.level)}
                </span>
                <span className="alert-type">{alert.type}</span>
                <span
                  className="severity-badge"
                  style={{ backgroundColor: getSeverityColor(alert.level) }}
                >
                  {alert.level.toUpperCase()}
                </span>
                {!alert.acknowledged && (
                  <button
                    onClick={() => handleAcknowledge(alert.id)}
                    className="btn-small"
                  >
                    Acknowledge
                  </button>
                )}
              </div>
              <div className="alert-message">{alert.message}</div>
              <div className="alert-details">
                <div className="detail-item">
                  <span className="detail-label">Created:</span>
                  <span className="detail-value">
                    {new Date(alert.created_at).toLocaleString()}
                  </span>
                </div>
                {alert.acknowledged && alert.acknowledged_at && (
                  <div className="detail-item">
                    <span className="detail-label">Acknowledged:</span>
                    <span className="detail-value">
                      {new Date(alert.acknowledged_at).toLocaleString()}
                    </span>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default AlertsPanel;
