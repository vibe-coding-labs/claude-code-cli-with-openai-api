import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';
import reportWebVitals from './reportWebVitals';
import axios from 'axios';
import { getApiOrigin } from './services/apiBase';
import { getToken } from './services/auth';

// ⚠️ 严禁随意修改端口！前后端端口配置需要保持一致！
// 后端固定端口：54988，前端固定端口：54989
// 前端由后端服务器提供时，使用相对路径即可（不需要设置 baseURL）
// axios.defaults.baseURL 默认为当前域名
axios.defaults.baseURL = getApiOrigin();

// Global auth interceptor — ensures ALL axios calls include the JWT token
axios.interceptors.request.use(
  (config) => {
    const token = getToken();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);
axios.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      const url = error.config?.url || '';
      if (!url.includes('/auth/login') && !url.includes('/auth/initialize')) {
        window.location.href = '/ui/login';
      }
    }
    return Promise.reject(error);
  }
);

const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
