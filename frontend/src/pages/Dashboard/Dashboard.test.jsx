import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dashboard } from './Dashboard';
import { getDashboardMetrics } from '../../services/api';

jest.mock('../../services/api');
// НЕТ jest.mock('react-router-dom')

jest.mock('recharts', () => ({
  ResponsiveContainer: ({ children }) => <div>{children}</div>,
  LineChart: () => <div>LineChart</div>,
  Line: () => null,
  BarChart: () => <div>BarChart</div>,
  Bar: () => null,
  PieChart: () => <div>PieChart</div>,
  Pie: () => null,
  Cell: () => null,
  CartesianGrid: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  Legend: () => null,
}));

jest.mock('../../components/ui', () => ({
  StateMessage: ({ title }) => <div>{title}</div>,
}));

jest.mock('./Dashboard.module.css', () => ({}));

const mockMetrics = {
  summary: {
    total_cases: 150,
    mean_risk: 0.55,
    total_posts: 5000,
    open_cases: 45,
    rumour_cases: 90,
    non_rumour_cases: 60,
  },
  engagement: {
    total_engagement: 28000,
    unique_accounts: 1200,
    verified_accounts: 300,
    post_count: 5000,
    total_likes: 15000,
    total_reposts: 8000,
  },
  cases_over_time: [],
  risk_distribution: { high: 45, medium: 50, low: 55 },
  top_events: [],
  scorer_performance: [],
  manipulation_stats: { total_analyzed: 4000, avg_manipulation_score: 0.35 },
  status_breakdown: [],
  label_breakdown: [],
  source_breakdown: [],
  dataset_breakdown: [],
  top_risk_cases: [],
};

describe('Dashboard', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    getDashboardMetrics.mockResolvedValue(mockMetrics);
  });

  it('shows loading state', () => {
    getDashboardMetrics.mockImplementation(() => new Promise(() => {}));
    render(<Dashboard />);
    expect(screen.getByText('Loading dashboard')).toBeTruthy();
  });

  it('shows error state', async () => {
    getDashboardMetrics.mockRejectedValue(new Error('Network error'));
    render(<Dashboard />);
    await waitFor(() => {
      expect(screen.getByText('Dashboard unavailable')).toBeTruthy();
    });
  });

  it('renders dashboard after loading', async () => {
    render(<Dashboard />);
    await waitFor(() => {
      expect(screen.getByText('Analytics Dashboard')).toBeTruthy();
    });
  });

  it('displays KPI cards', async () => {
    render(<Dashboard />);
    await waitFor(() => {
      expect(screen.getByText('Cases')).toBeTruthy();
      expect(screen.getByText('Mean risk')).toBeTruthy();
      expect(screen.getByText('Engagement')).toBeTruthy();
    });
  });

  it('displays case count', async () => {
    render(<Dashboard />);
    await waitFor(() => {
      expect(screen.getByText('150')).toBeTruthy();
    });
  });
});