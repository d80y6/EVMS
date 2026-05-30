import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

function parseJWT(token: string): { username: string; role: string } | null {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = JSON.parse(atob(parts[1]));
    return {
      username: payload.username || '',
      role: payload.role || 'viewer',
    };
  } catch {
    return null;
  }
}

interface AuthContextType {
  token: string | null;
  isAuthenticated: boolean;
  username: string;
  role: string;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  isLoading: boolean;
  error: string | null;
}

const AuthContext = createContext<AuthContextType>({
  token: null,
  isAuthenticated: false,
  username: '',
  role: 'viewer',
  login: async () => {},
  logout: () => {},
  isLoading: false,
  error: null,
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('auth_token'));
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const claims = token ? parseJWT(token) : null;
  const username = claims?.username || '';
  const role = claims?.role || 'viewer';

  useEffect(() => {
    if (token) {
      localStorage.setItem('auth_token', token);
    } else {
      localStorage.removeItem('auth_token');
    }
  }, [token]);

  const login = useCallback(async (loginUsername: string, password: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await api.login(loginUsername, password);
      setToken(res.token);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Login failed';
      setError(msg);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  const logout = useCallback(() => {
    setToken(null);
  }, []);

  return (
    <AuthContext.Provider
      value={{
        token,
        isAuthenticated: !!token,
        username,
        role,
        login,
        logout,
        isLoading,
        error,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
