import { render, screen, waitFor } from '@testing-library/react';
import { CasesQueue } from './CasesQueue';
import { getCases, getCasesSummary } from '../../services/api';

jest.mock('../../services/api');

// ЭТО КЛЮЧЕВОЙ МОМЕНТ - мокаем модуль ДО импорта компонента
jest.mock('react-router-dom', () => ({
  useNavigate: () => jest.fn(),
}));

jest.mock('../../components/ui', () => ({
  ComponentMini: () => <div>Components</div>,
  RiskPill: ({ level }) => <span>{level}</span>,
  ScoreBar: () => <div>Score</div>,
  SelectFilter: ({ label, value, onChange, options }) => (
    <div>
      <label>{label}</label>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map(opt => <option key={opt}>{opt}</option>)}
      </select>
    </div>
  ),
  Sparkline: () => <div>Sparkline</div>,
  StateMessage: ({ title }) => <div>{title}</div>,
  SummaryTile: ({ label, value }) => <div>{label}: {value}</div>,
}));

jest.mock('../../constants', () => ({ DEFAULT_SCORER: 'default' }));
jest.mock('../../utils/format', () => ({
  formatScore: (v) => (v || 0).toFixed(2),
  summarize: () => ({ high: 0, medium: 0, low: 0, mean: 0 }),
}));
jest.mock('./CasesQueue.module.css', () => ({}));

const mockCases = {
  items: [
    {
      id: 1,
      title: 'Test Case 1',
      event_name: 'Event A',
      dataset_name: 'pheme',
      dataset_split: 'train',
      status: 'open',
      risk_score: 0.85,
      risk_level: 'high',
      post_count: 100,
      duration_min: 120,
      evidence: [{ key: 'evidence1' }],
      timeline: [1, 2, 3],
      scores: {},
    },
  ],
  page: 1,
};

const mockSummary = {
  total_cases: 100,
  high_risk: 30,
  medium_risk: 40,
  low_risk: 30,
  mean_risk: 0.55,
  pages: 5,
};

describe('CasesQueue', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    getCases.mockResolvedValue(mockCases);
    getCasesSummary.mockResolvedValue(mockSummary);
  });

  it('shows loading state', () => {
    getCases.mockImplementation(() => new Promise(() => {}));
    render(<CasesQueue />);
    expect(screen.getByText('Loading cases')).toBeTruthy();
  });

  it('shows error state', async () => {
    getCases.mockRejectedValue(new Error('Network error'));
    render(<CasesQueue />);
    await waitFor(() => {
      expect(screen.getByText('Backend unavailable')).toBeTruthy();
    });
  });

  it('renders cases after loading', async () => {
    render(<CasesQueue />);
    await waitFor(() => {
      expect(screen.getByText('Test Case 1')).toBeTruthy();
    });
  });

  it('displays summary tiles', async () => {
    render(<CasesQueue />);
    await waitFor(() => {
      expect(screen.getByText('Cases: 100')).toBeTruthy();
    });
  });
});