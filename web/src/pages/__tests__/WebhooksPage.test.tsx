import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import WebhooksPage from '../WebhooksPage';
import * as client from '../../api/client';

vi.mock('../../api/client', () => ({
  api: {
    listWebhooks: vi.fn(),
    createWebhook: vi.fn(),
    updateWebhook: vi.fn(),
    deleteWebhook: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

const mockWebhooks = [
  { id: 'wh-1', name: 'Slack Alerts', url: 'https://hooks.slack.com/xxx', event_types: ['motion', 'line_cross'], camera_ids: [], enabled: true },
  { id: 'wh-2', name: 'Email Log', url: 'https://hooks.example.com/email', event_types: ['error'], camera_ids: ['cam-1'], enabled: false },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.api.listWebhooks).mockResolvedValue({ webhooks: mockWebhooks });
});

describe('WebhooksPage', () => {
  it('shows loading state initially', () => {
    vi.mocked(client.api.listWebhooks).mockImplementation(() => new Promise(() => {}));
    render(<WebhooksPage />);
    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });

  it('shows empty state when no webhooks', async () => {
    vi.mocked(client.api.listWebhooks).mockResolvedValue({ webhooks: [] });
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('No webhooks configured.')).toBeInTheDocument();
    });
  });

  it('renders webhook names and URLs', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
    expect(screen.getByText('Email Log')).toBeInTheDocument();
    expect(screen.getByText('https://hooks.slack.com/xxx')).toBeInTheDocument();
    expect(screen.getByText('https://hooks.example.com/email')).toBeInTheDocument();
  });

  it('opens create form when clicking + Add Webhook', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText('+ Add Webhook'));
    expect(screen.getByPlaceholderText('Name')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('URL (https://...)')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Event types (comma-separated)')).toBeInTheDocument();
    expect(screen.getByText('Create')).toBeInTheDocument();
  });

  it('pre-fills form when clicking Edit', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
    const editButtons = screen.getAllByText('Edit');
    await userEvent.click(editButtons[0]);
    expect(screen.getByPlaceholderText('Name')).toHaveValue('Slack Alerts');
    expect(screen.getByPlaceholderText('URL (https://...)')).toHaveValue('https://hooks.slack.com/xxx');
    expect(screen.getByText('Update')).toBeInTheDocument();
  });

  it('calls deleteWebhook when clicking Delete', async () => {
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
    const deleteButtons = screen.getAllByText('Delete');
    await userEvent.click(deleteButtons[0]);
    expect(vi.mocked(client.api.deleteWebhook)).toHaveBeenCalledWith('wh-1');
    await waitFor(() => {
      expect(screen.getByText('Webhook deleted')).toBeInTheDocument();
    });
  });

  it('shows error banner when listWebhooks fails', async () => {
    vi.mocked(client.api.listWebhooks).mockRejectedValue(new Error('Network error'));
    render(<WebhooksPage />);
    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument();
    });
  });
});
