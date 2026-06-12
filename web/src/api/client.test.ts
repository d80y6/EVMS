import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
  setAuthToken,
  getAuthToken,
  setCSRFToken,
  getCSRFToken,
  apiClient,
} from './client';

beforeEach(() => {
  setAuthToken(null);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('setAuthToken / getAuthToken', () => {
  it('should default to null', () => {
    expect(getAuthToken()).toBeNull();
  });

  it('should set and get a token', () => {
    setAuthToken('test-token');
    expect(getAuthToken()).toBe('test-token');
  });

  it('should clear token when set to null', () => {
    setAuthToken('test-token');
    setAuthToken(null);
    expect(getAuthToken()).toBeNull();
  });
});

describe('setCSRFToken / getCSRFToken', () => {
  it('should default to null', () => {
    expect(getCSRFToken()).toBeNull();
  });

  it('should set and get a CSRF token', () => {
    setCSRFToken('csrf-token-value');
    expect(getCSRFToken()).toBe('csrf-token-value');
  });
});

describe('apiClient.fetch', () => {
  it('should include Authorization header when auth token is set', async () => {
    setAuthToken('test-jwt');
    const mockFetch = vi.fn().mockResolvedValue(new Response('ok'));
    vi.stubGlobal('fetch', mockFetch);

    await apiClient.fetch('/api/test');

    const [, options] = mockFetch.mock.calls[0];
    expect(options.headers).toEqual(
      expect.objectContaining({ Authorization: 'Bearer test-jwt' })
    );

    vi.unstubAllGlobals();
  });

  it('should not include Authorization header when no token', async () => {
    setAuthToken(null);
    const mockFetch = vi.fn().mockResolvedValue(new Response('ok'));
    vi.stubGlobal('fetch', mockFetch);

    await apiClient.fetch('/api/test');

    const [, options] = mockFetch.mock.calls[0];
    expect(options.headers).not.toHaveProperty('Authorization');

    vi.unstubAllGlobals();
  });
});
