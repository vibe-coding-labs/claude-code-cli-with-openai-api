import React, { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { Spin } from 'antd';
import { checkInitialized, isAuthenticated } from '../services/auth';

interface ProtectedRouteProps {
  children: React.ReactElement;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const [loading, setLoading] = useState(true);
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    const checkStatus = async () => {
      const isInit = await checkInitialized();
      setInitialized(isInit);
      setLoading(false);
    };
    checkStatus();
  }, []);

  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '100vh',
        }}
      >
        <Spin size="large">
          <div style={{ padding: '50px 0' }}>加载中...</div>
        </Spin>
      </div>
    );
  }

  // If not initialized, redirect to initialize page
  if (!initialized) {
    return <Navigate to="/ui/initialize" replace />;
  }

  // If initialized but not authenticated, redirect to login
  if (!isAuthenticated()) {
    return <Navigate to="/ui/login" replace />;
  }

  return children;
};

export default ProtectedRoute;
