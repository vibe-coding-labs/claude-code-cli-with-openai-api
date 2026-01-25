import React, { useEffect, useState } from 'react';
import { loadBalancerApi, EnhancedStats } from '../services/loadBalancerApi';
import './LoadBalancerDetail.css';

interface EnhancedStatsPanelProps {
  loadBalancerId: string;
}

const EnhancedStatsPanel: React.FC<EnhancedStatsPanelProps> = ({ loadBalancerId }) => {
  const [stats, setStats] = useState<EnhancedStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [timeRange, setTimeRange] = useState<'1h' | '24h' | '7d'>('1h');

  const fetchStats = async () => {
    try {
      setLoading(true);
      const data = await loadBalancerApi.getEnhancedStats(loadBalancerId, timeRange);
      setStats(data);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch statistics');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
    // Refresh every 30 seconds
    const interval = setInterval(fetchStats, 30000);
    return () => clearInterval(interval);
  }, [loadBalancerId, timeRange]);

  const formatNumber = (num: number) => {
    if (num >= 1000000) return (num / 1000000).toFixed(2) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(2) + 'K';
    return num.toString();
  };

  const formatDuration = (ms: number) => {
    if (ms < 1) return '<1ms';
    if (ms < 1000) return ms.toFixed(0) + 'ms';
    return (ms / 1000).toFixed(2) + 's';
  };

  const calculateSuccessRate = () => {
    if (!stats || stats.total_requests === 0) return 0;
    return ((stats.success_requests / stats.total_requests) * 100).toFixed(2);
  };

  if (loading && !stats) {
    return <div className="panel">Loading statistics...</div>;
  }

  if (error) {
    return (
      <div className="panel">
        <div className="error-message">Error: {error}</div>
        <button onClick={fetchStats}>Retry</button>
      </div>
    );
  }

  if (!stats) {
    return null;
  }

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Statistics</h3>
        <div className="time-range-selector">
          <button
            className={timeRange === '1h' ? 'btn-filter-active' : 'btn-filter'}
            onClick={() => setTimeRange('1h')}
          >
            1 Hour
          </button>
          <button
            className={timeRange === '24h' ? 'btn-filter-active' : 'btn-filter'}
            onClick={() => setTimeRange('24h')}
          >
            24 Hours
          </button>
          <button
            className={timeRange === '7d' ? 'btn-filter-active' : 'btn-filter'}
            onClick={() => setTimeRange('7d')}
          >
            7 Days
          </button>
        </div>
      </div>

      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-label">Total Requests</div>
          <div className="stat-value">{formatNumber(stats.total_requests)}</div>
        </div>
        <div className="stat-card" style={{ borderColor: '#10b981' }}>
          <div className="stat-label">Successful</div>
          <div className="stat-value" style={{ color: '#10b981' }}>
            {formatNumber(stats.success_requests)}
          </div>
        </div>
        <div className="stat-card" style={{ borderColor: '#ef4444' }}>
          <div className="stat-label">Failed</div>
          <div className="stat-value" style={{ color: '#ef4444' }}>
            {formatNumber(stats.failed_requests)}
          </div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Success Rate</div>
          <div className="stat-value">{calculateSuccessRate()}%</div>
        </div>
      </div>

      <div className="stats-section">
        <h4>Response Times</h4>
        <div className="stats-grid">
          <div className="stat-card">
            <div className="stat-label">Average</div>
            <div className="stat-value">{formatDuration(stats.avg_response_time_ms)}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">P50</div>
            <div className="stat-value">{formatDuration(stats.p50_response_time_ms)}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">P95</div>
            <div className="stat-value">{formatDuration(stats.p95_response_time_ms)}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">P99</div>
            <div className="stat-value">{formatDuration(stats.p99_response_time_ms)}</div>
          </div>
        </div>
      </div>

      {stats.node_stats && stats.node_stats.length > 0 && (
        <div className="stats-section">
          <h4>Node Statistics</h4>
          <div className="node-stats-table">
            <table>
              <thead>
                <tr>
                  <th>Config ID</th>
                  <th>Requests</th>
                  <th>Success Rate</th>
                  <th>Avg Response</th>
                </tr>
              </thead>
              <tbody>
                {stats.node_stats.map((node) => (
                  <tr key={node.config_id}>
                    <td>{node.config_id}</td>
                    <td>{formatNumber(node.request_count)}</td>
                    <td style={{ color: node.success_rate > 0.9 ? '#10b981' : '#ef4444' }}>
                      {(node.success_rate * 100).toFixed(2)}%
                    </td>
                    <td>{formatDuration(node.avg_response_time_ms)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
};

export default EnhancedStatsPanel;
