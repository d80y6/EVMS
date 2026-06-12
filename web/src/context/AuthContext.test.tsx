import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthProvider, useAuth } from './AuthContext';
import * as client from '../api/client';

vi.mock('../api/client', () => ({
  api: {
    login: vi.fn(),
    verifyMFA: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

function TestConsumer() {
  const auth = useAuth();
  return (
    <div>
      <span data-testid="isAuthenticated">{String(auth.isAuthenticated)}</span>
      <span data-testid="username">{auth.username}</span>
      <span data-testid="role">{auth.role}</span>
      <span data-testid="error">{auth.error || ''}</span>
      <span data-testid="isLoading">{String(auth.isLoading)}</span>
      <span data-testid="mfaRequired">{String(auth.mfaRequired)}</span>
      <button data-testid="login-btn" onClick={() => auth.login('testuser', 'pass')}>
        Login
      </button>
      <button data-testid="logout-btn" onClick={() => auth.logout()}>
        Logout
      </button>
    </div>
  );
}

function renderWithProvider() {
  return render(
    <AuthProvider>
      <TestConsumer />
    </AuthProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  const mockedGetAuthToken = vi.mocked(client.getAuthToken);
  mockedGetAuthToken.mockReturnValue(null);
});

describe('AuthProvider', () => {
  it('renders children', () => {
    render(
      <AuthProvider>
        <div data-testid="child">Hello</div>
      </AuthProvider>
    );
    expect(screen.getByTestId('child')).toHaveTextContent('Hello');
  });

  it('provides default unauthenticated state', () => {
    renderWithProvider();
    expect(screen.getByTestId('isAuthenticated')).toHaveTextContent('false');
    expect(screen.getByTestId('username')).toHaveTextContent('');
    expect(screen.getByTestId('role')).toHaveTextContent('viewer');
    expect(screen.getByTestId('error')).toHaveTextContent('');
    expect(screen.getByTestId('isLoading')).toHaveTextContent('false');
    expect(screen.getByTestId('mfaRequired')).toHaveTextContent('false');
  });
});

describe('login', () => {
  it('sets token on successful login', async () => {
    const fakeToken = 'eyJhbGciOiJIUzI1NiJ9.eyJ1c2VybmFtZSI6ImFkbWluIiwicm9sZSI6ImFkbWluIn0.signature';
    const mockedLogin = vi.mocked(client.api.login);
    mockedLogin.mockResolvedValue({ token: fakeToken });
    const mockedFetchCSRF = vi.mocked(client.fetchCSRFToken);
    mockedFetchCSRF.mockResolvedValue('');

    renderWithProvider();

    await userEvent.click(screen.getByTestId('login-btn'));

    expect(mockedLogin).toHaveBeenCalledWith('testuser', 'pass');
    expect(mockedFetchCSRF).toHaveBeenCalled();
  });

  it('sets error on failed login', async () => {
    const mockedLogin = vi.mocked(client.api.login);
    mockedLogin.mockRejectedValue(new Error('Invalid credentials'));

    function ErrorConsumer() {
      const auth = useAuth();
      return (
        <div>
          <span data-testid="error">{auth.error || ''}</span>
          <button
            data-testid="login-btn"
            onClick={async () => {
              try {
                await auth.login('testuser', 'pass');
              } catch {
                // expected
              }
            }}
          >
            Login
          </button>
        </div>
      );
    }

    render(
      <AuthProvider>
        <ErrorConsumer />
      </AuthProvider>
    );

    await userEvent.click(screen.getByTestId('login-btn'));

    await waitFor(() => {
      expect(screen.getByTestId('error')).toHaveTextContent('Invalid credentials');
    });
  });

  it('sets mfaRequired when MFA is needed', async () => {
    const mockedLogin = vi.mocked(client.api.login);
    mockedLogin.mockResolvedValue({ mfa_required: true, mfa_token: 'mfa-token', token: '' });

    renderWithProvider();

    await userEvent.click(screen.getByTestId('login-btn'));

    expect(screen.getByTestId('mfaRequired')).toHaveTextContent('true');
  });
});

describe('logout', () => {
  it('clears token on logout', async () => {
    const fakeToken = 'eyJhbGciOiJIUzI1NiJ9.eyJ1c2VybmFtZSI6ImFkbWluIiwicm9sZSI6ImFkbWluIn0.signature';
    const mockedLogin = vi.mocked(client.api.login);
    mockedLogin.mockResolvedValue({ token: fakeToken });
    const mockedFetchCSRF = vi.mocked(client.fetchCSRFToken);
    mockedFetchCSRF.mockResolvedValue('');

    renderWithProvider();

    await userEvent.click(screen.getByTestId('login-btn'));
    await userEvent.click(screen.getByTestId('logout-btn'));

    expect(screen.getByTestId('isAuthenticated')).toHaveTextContent('false');
    expect(screen.getByTestId('mfaRequired')).toHaveTextContent('false');
  });
});

describe('JWT parsing via AuthProvider', () => {
  it('parses username and role from token on initialization', () => {
    const payload = btoa(JSON.stringify({ username: 'jane', role: 'editor' }));
    const token = `header.${payload}.signature`;
    const mockedGetAuthToken = vi.mocked(client.getAuthToken);
    mockedGetAuthToken.mockReturnValue(token);

    renderWithProvider();

    expect(screen.getByTestId('username')).toHaveTextContent('jane');
    expect(screen.getByTestId('role')).toHaveTextContent('editor');
    expect(screen.getByTestId('isAuthenticated')).toHaveTextContent('true');
  });

  it('handles malformed token gracefully', () => {
    const mockedGetAuthToken = vi.mocked(client.getAuthToken);
    mockedGetAuthToken.mockReturnValue('not-a-valid-jwt');

    renderWithProvider();

    expect(screen.getByTestId('username')).toHaveTextContent('');
    expect(screen.getByTestId('role')).toHaveTextContent('viewer');
    expect(screen.getByTestId('isAuthenticated')).toHaveTextContent('true');
  });
});
