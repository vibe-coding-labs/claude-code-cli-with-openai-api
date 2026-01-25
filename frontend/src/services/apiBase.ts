// ⚠️ 严禁随意修改！后端固定端口54988，前端固定端口54989
// 开发模式下使用绝对路径，避免 /ui 前缀导致 API 404
export const getApiOrigin = (): string => {
  if (process.env.NODE_ENV === 'development') {
    return 'http://localhost:54988';
  }

  return '';
};
