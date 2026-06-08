import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { api, fetchCSRFToken } from '../api/client';

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
  mfaRequired: boolean;
  mfaToken: string | null;
  verifyMFA: (code: string) => Promise<void>;
  cancelMFA: () => void;
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
  mfaRequired: false,
  mfaToken: null,
  verifyMFA: async () => {},
  cancelMFA: () => {},
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('auth_token'));
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mfaRequired, setMFARequired] = useState(false);
  const [mfaToken, setMFAToken] = useState<string | null>(null);

  const claims = token ? parseJWT(token) : null;
  const username = claims?.username || '';
  const role = claims?.role || 'viewer';

  useEffect(() => {
    if (token) {
      localStorage.setItem('auth_token', token);
      fetchCSRFToken();
    } else {
      localStorage.removeItem('auth_token');
    }
  }, [token]);

  const login = useCallback(async (loginUsername: string, password: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await api.login(loginUsername, password);
      if (res.mfa_required) {
        setMFARequired(true);
        setMFAToken(res.mfa_token || null);
        return;
      }
      setToken(res.token);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Login failed';
      setError(msg);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  const verifyMFA = useCallback(async (code: string) => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await api.verifyMFA(code);
      if (res.token) {
        setToken(res.token);
        setMFARequired(false);
        setMFAToken(null);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'MFA verification failed';
      setError(msg);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  const cancelMFA = useCallback(() => {
    setMFARequired(false);
    setMFAToken(null);
    setError(null);
  }, []);

  const logout = useCallback(() => {
    setToken(null);
    setMFARequired(false);
    setMFAToken(null);
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
        mfaRequired,
        mfaToken,
        verifyMFA,
        cancelMFA,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
