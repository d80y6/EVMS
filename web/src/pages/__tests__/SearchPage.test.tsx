import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SearchPage from '../SearchPage';
import * as client from '../../api/client';

vi.mock('../../api/client', () => ({
  api: {
    smartSearch: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

const mockResults = [
  {
    id: 'evt-1',
    camera_id: 'cam-1',
    event_time: '2024-06-15T10:30:00Z',
    object_type: 'person',
    confidence: 0.95,
    track_id: 'track-1',
    thumbnail: '/thumbnails/evt-1.jpg',
  },
  {
    id: 'evt-2',
    camera_id: 'cam-2',
    event_time: '2024-06-15T10:31:00Z',
    object_type: 'vehicle',
    confidence: 0.87,
    track_id: 'track-2',
    thumbnail: '/thumbnails/evt-2.jpg',
  },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SearchPage', () => {
  it('shows filter inputs', () => {
    render(<SearchPage />);
    expect(screen.getByPlaceholderText('Object Type')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Camera ID')).toBeInTheDocument();
    expect(screen.getByText('Search')).toBeInTheDocument();
    expect(screen.getByText('Minimum Confidence')).toBeInTheDocument();
  });

  it('shows loading spinner during search', async () => {
    vi.mocked(client.api.smartSearch).mockImplementation(() => new Promise(() => {}));
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    expect(screen.getByText('Searching...')).toBeInTheDocument();
  });

  it('shows empty results message when no results found', async () => {
    vi.mocked(client.api.smartSearch).mockResolvedValue({ results: [], total: 0 });
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText(/No results found/)).toBeInTheDocument();
    });
  });

  it('shows error message when search fails', async () => {
    vi.mocked(client.api.smartSearch).mockRejectedValue(new Error('Search service unavailable'));
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText('Search service unavailable')).toBeInTheDocument();
    });
  });

  it('renders search results with object_type and event_time', async () => {
    vi.mocked(client.api.smartSearch).mockResolvedValue({ results: mockResults, total: 2 });
    render(<SearchPage />);
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getByText('person')).toBeInTheDocument();
    });
    expect(screen.getByText('vehicle')).toBeInTheDocument();
    expect(screen.getByText('2024-06-15T10:30:00Z')).toBeInTheDocument();
    expect(screen.getByText('2024-06-15T10:31:00Z')).toBeInTheDocument();
  });

  it('shows correct stats counts', async () => {
    vi.mocked(client.api.smartSearch).mockResolvedValue({ results: mockResults, total: 2 });
    render(<SearchPage />);
    expect(screen.getByText('Results')).toBeInTheDocument();
    expect(screen.getByText('Cameras')).toBeInTheDocument();
    expect(screen.getByText('Avg Confidence')).toBeInTheDocument();
    await userEvent.click(screen.getByText('Search'));
    await waitFor(() => {
      expect(screen.getAllByText('2').length).toBeGreaterThan(0);
    });
  });
});
