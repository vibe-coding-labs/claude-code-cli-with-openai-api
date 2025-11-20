const TOKEN_KEY = 'auth_token';
const USERNAME_KEY = 'username';

export const auth = {
  // 保存token
  setToken(token: string, username: string) {
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.setItem(USERNAME_KEY, username);
  },

  // 获取token
  getToken(): string | null {
    return localStorage.getItem(TOKEN_KEY);
  },

  // 获取用户名
  getUsername(): string | null {
    return localStorage.getItem(USERNAME_KEY);
  },

  // 清除token
  clearToken() {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USERNAME_KEY);
  },

  // 检查是否已登录
  isAuthenticated(): boolean {
    return !!this.getToken();
  },
};
