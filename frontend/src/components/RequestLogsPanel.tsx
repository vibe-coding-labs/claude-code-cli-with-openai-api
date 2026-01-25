import React, { useEffect, useState } from 'react';
import { loadBalancerApi, RequestLog } from '../services/loadBalancerApi';
import './LoadBalancerDetail.css';

interface RequestLogsPanelProps {
  loadBalancerId: string;
}

const RequestLogsPanel: React.FC<RequestLogsPanelProps> = ({ loadBalancerId }) => {
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const pageSize = 50;

  const fetchLogs = async (pageNum: number, reset: boolean = false) => {
    try {
      setLoading(true);
      const offset = (pageNum - 1) * pageSize;
      const data = await loadBalancerApi.getRequestLogs(
        loadBalancerId,
        pageSize,
        offset
      );
      
      if (reset) {
        setLogs(data.logs || []);
      } else {
        setLogs((prev) => [...prev, ...(data.logs || [])]);
      }
      
      setHasMore((data.logs || []).length === pageSize);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Failed to fetch request logs');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setPage(1);
    fetchLogs(1, true);
  }, [loadBalancerId]);

  const handleLoadMore = () => {
    const nextPage = page + 1;
    setPage(nextPage);
    fetchLogs(nextPage, false);
  };

  const getStatusColor = (success: boolean) => {
    return success ? '#10b981' : '#ef4444';
  };

  const getStatusText = (success: boolean) => {
    return success ? 'SUCCESS' : 'FAILED';
  };

  const formatDuration = (ms: number) => {
    if (ms < 1) return '<1ms';
    if (ms < 1000) return ms.toFixed(0) + 'ms';
    return (ms / 1000).toFixed(2) + 's';
  };

  if (loading && logs.length === 0) {
    return <div className="panel">Loading request logs...</div>;
  }

  if (error) {
    return (
      <div className="panel">
        <div className="error-message">Error: {error}</div>
        <button onClick={() => fetchLogs(1, true)}>Retry</button>
      </div>
    );
  }

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Request Logs</h3>
      </div>

      {logs.length === 0 ? (
        <div className="empty-state">No request logs found</div>
      ) : (
        <>
          <div className="logs-list">
            {logs.map((log) => (
              <div key={log.id} className="log-item">
                <div className="log-header">
                  <span
                    className="status-badge"
                    style={{ backgroundColor: getStatusColor(log.success) }}
                  >
                    {getStatusText(log.success)}
                  </span>
                  <span className="log-timestamp">
                    {new Date(log.request_time).toLocaleString()}
                  </span>
                  <span className="log-duration">{formatDuration(log.duration_ms)}</span>
                </div>
                <div className="log-details">
                  <div className="detail-row">
                    <span className="detail-label">Config ID:</span>
                    <span className="detail-value">{log.selected_config_id}</span>
                  </div>
                  <div className="detail-row">
                    <span className="detail-label">Method:</span>
                    <span className="detail-value">{log.request_summary || 'N/A'}</span>
                  </div>
                  <div className="detail-row">
                    <span className="detail-label">Status Code:</span>
                    <span className="detail-value">{log.status_code}</span>
                  </div>
                  {log.retry_count > 0 && (
                    <div className="detail-row">
                      <span className="detail-label">Retries:</span>
                      <span className="detail-value">{log.retry_count}</span>
                    </div>
                  )}
                  {log.error_message && (
                    <div className="detail-row error">
                      <span className="detail-label">Error:</span>
                      <span className="detail-value">{log.error_message}</span>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
          {hasMore && (
            <div className="load-more-container">
              <button onClick={handleLoadMore} disabled={loading} className="btn-secondary">
                {loading ? 'Loading...' : 'Load More'}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
};

export default RequestLogsPanel;
