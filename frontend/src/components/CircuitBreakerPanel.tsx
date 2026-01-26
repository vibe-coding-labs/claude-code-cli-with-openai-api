/* eslint-disable react-hooks/exhaustive-deps */
import React, { useEffect, useState } from 'react';
import { loadBalancerApi, CircuitBreakerState } from '../services/loadBalancerApi';
import './LoadBalancerDetail.css';

interface CircuitBreakerPanelProps {
  loadBalancerId: string;
}

const CircuitBreakerPanel: React.FC<CircuitBreakerPanelProps> = ({ loadBalancerId }) => {
  const [states, setStates] = useState<CircuitBreakerState[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchCircuitBreakerStates = async () => {
    try {
      setLoading(true);
      const data = await loadBalancerApi.getCircuitBreakers(loadBalancerId);
      setStates(data.states || []);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch circuit breaker states');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCircuitBreakerStates();
    // Refresh every 30 seconds
    const interval = setInterval(fetchCircuitBreakerStates, 30000);
    return () => clearInterval(interval);
  }, [loadBalancerId]);

  const handleReset = async (configId: string) => {
    try {
      await loadBalancerApi.resetCircuitBreaker(loadBalancerId, configId);
      // Refresh after reset
      setTimeout(fetchCircuitBreakerStates, 1000);
    } catch (err: any) {
      setError(err.message || 'Failed to reset circuit breaker');
    }
  };

  const getStateColor = (state: string) => {
    switch (state) {
      case 'closed':
        return '#10b981';
      case 'open':
        return '#ef4444';
      case 'half_open':
        return '#f59e0b';
      default:
        return '#6b7280';
    }
  };

  const getStateIcon = (state: string) => {
    switch (state) {
      case 'closed':
        return '●';
      case 'open':
        return '○';
      case 'half_open':
        return '◐';
      default:
        return '?';
    }
  };

  if (loading && states.length === 0) {
    return <div className="panel">Loading circuit breaker states...</div>;
  }

  if (error) {
    return (
      <div className="panel">
        <div className="error-message">Error: {error}</div>
        <button onClick={fetchCircuitBreakerStates}>Retry</button>
      </div>
    );
  }

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Circuit Breakers</h3>
      </div>

      {states.length === 0 ? (
        <div className="empty-state">No circuit breaker data available</div>
      ) : (
        <div className="circuit-breaker-list">
          {states.map((state) => (
            <div key={state.config_id} className="circuit-breaker-item">
              <div className="status-header">
                <span
                  className="status-icon"
                  style={{ color: getStateColor(state.state) }}
                >
                  {getStateIcon(state.state)}
                </span>
                <span className="config-id">{state.config_id}</span>
                <span className="status-badge" style={{ color: getStateColor(state.state) }}>
                  {state.state.toUpperCase().replace('_', ' ')}
                </span>
                {state.state !== 'closed' && (
                  <button
                    onClick={() => handleReset(state.config_id)}
                    className="btn-small"
                  >
                    Reset
                  </button>
                )}
              </div>
              <div className="status-details">
                <div className="detail-item">
                  <span className="detail-label">Failure Count:</span>
                  <span className="detail-value">{state.failure_count}</span>
                </div>
                <div className="detail-item">
                  <span className="detail-label">Success Count:</span>
                  <span className="detail-value">{state.success_count}</span>
                </div>
                <div className="detail-item">
                  <span className="detail-label">Last State Change:</span>
                  <span className="detail-value">
                    {new Date(state.last_state_change).toLocaleString()}
                  </span>
                </div>
                {state.state === 'open' && state.next_retry_time && (
                  <div className="detail-item">
                    <span className="detail-label">Next Retry:</span>
                    <span className="detail-value">
                      {new Date(state.next_retry_time).toLocaleString()}
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

export default CircuitBreakerPanel;
