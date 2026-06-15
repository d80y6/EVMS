import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SettingsPage from '../SettingsPage';
import * as client from '../../api/client';

vi.mock('../../api/client', () => ({
  api: {
    getCameras: vi.fn(),
    listTours: vi.fn(),
    startTour: vi.fn(),
    stopTour: vi.fn(),
    deleteTour: vi.fn(),
    createTour: vi.fn(),
    updateCameraConfig: vi.fn(),
    changePassword: vi.fn(),
    setRelayState: vi.fn(),
  },
  setAuthToken: vi.fn(),
  getAuthToken: vi.fn(() => null),
  fetchCSRFToken: vi.fn(),
}));

vi.mock('../../components/FloorPlanView', () => ({
  FloorPlanView: () => null,
}));

const mockCameras = [
  { id: 'cam-1', site_id: 'site-1', name: 'Front Door', description: '', connection_url: '', substream_url: '', status: 'online', ptz_protocol: '', retention_days: 14, config: '' },
  { id: 'cam-2', site_id: 'site-1', name: 'Back Yard', description: '', connection_url: '', substream_url: '', status: 'online', ptz_protocol: 'onvif', retention_days: 30, config: '' },
];

const mockTours = [
  { id: 'tour-1', name: 'Morning Patrol', enabled: false, steps: [{ camera_id: 'cam-1', dwell_seconds: 10 }], interval: 30, created_at: '2024-01-01T00:00:00Z' },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.api.getCameras).mockResolvedValue({ cameras: mockCameras });
  vi.mocked(client.api.listTours).mockResolvedValue({ tours: mockTours });
});

describe('SettingsPage', () => {
  it('shows loading state initially', () => {
    render(<SettingsPage />);
    expect(screen.getByText('Loading settings...')).toBeInTheDocument();
  });

  it('renders camera names after loading', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getAllByText('Front Door')[0]).toBeInTheDocument();
    });
    expect(screen.getAllByText('Back Yard')[0]).toBeInTheDocument();
  });

  it('shows error when getCameras fails', async () => {
    vi.mocked(client.api.getCameras).mockRejectedValue(new Error('Failed to fetch'));
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByText('Failed to fetch')).toBeInTheDocument();
    });
  });

  it('renders retention range sliders for each camera', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      const sliders = screen.getAllByRole('slider');
      expect(sliders).toHaveLength(2);
    });
  });

  it('renders tour list', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByText('Morning Patrol')).toBeInTheDocument();
    });
    expect(screen.getByText('(1 steps, 30s interval)')).toBeInTheDocument();
  });

  it('opens new tour dialog when clicking + New Tour', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.queryByText('Loading settings...')).not.toBeInTheDocument();
    });
    await userEvent.click(screen.getByText('+ New Tour'));
    expect(screen.getByText('New Tour')).toBeInTheDocument();
    expect(screen.getByText('Tour Name')).toBeInTheDocument();
  });

  it('renders password change form', async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.queryByText('Loading settings...')).not.toBeInTheDocument();
    });
    expect(screen.getByText('Change Password')).toBeInTheDocument();
    expect(screen.getByText('Current Password')).toBeInTheDocument();
    expect(screen.getByText('New Password')).toBeInTheDocument();
    expect(screen.getByText('Update Password')).toBeInTheDocument();
  });
});
