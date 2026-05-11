// Accounts.test.jsx
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Accounts } from './Accounts';
import { getAccounts } from '../../services/api';

// Mock the API module
jest.mock('../../services/api');

// Mock the CSS module
jest.mock('./Accounts.module.css', () => ({}));

describe('Accounts Component', () => {
  const mockAccounts = {
    items: [
      {
        id: '1',
        username: 'testuser1',
        display_name: 'Test User 1',
        is_verified: true,
        is_bot: false,
        first_seen_at: '2024-01-15T10:00:00Z',
        dataset_post_count: 150,
        root_post_count: 30,
        case_count: 12,
        high_risk_case_count: 5,
        total_engagement: 25000,
        max_manipulation: 0.85,
        avg_risk_score: 0.65,
        followers_count: 10000,
        following_count: 500,
      },
      {
        id: '2',
        username: 'testuser2',
        display_name: 'Test User 2',
        is_verified: false,
        is_bot: true,
        first_seen_at: '2024-02-20T14:30:00Z',
        dataset_post_count: 75,
        root_post_count: 15,
        case_count: 8,
        high_risk_case_count: 3,
        total_engagement: 5000,
        max_manipulation: 0.45,
        avg_risk_score: 0.35,
        followers_count: 2500,
        following_count: 1200,
      },
    ],
    total: 2,
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('Initial Render', () => {
    it('should show loading state initially', () => {
      getAccounts.mockImplementation(() => new Promise(() => {}));
      render(<Accounts />);
      
      expect(screen.getByText(/Loading accounts/i)).toBeTruthy();
      expect(screen.getByText(/Aggregating account-level signals/i)).toBeTruthy();
    });

    it('should display accounts after loading', async () => {
      getAccounts.mockResolvedValue(mockAccounts);
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText(/@testuser1/)).toBeTruthy();
        expect(screen.getByText(/@testuser2/)).toBeTruthy();
      });
    });

    it('should display account details correctly', async () => {
      getAccounts.mockResolvedValue(mockAccounts);
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText('150')).toBeTruthy();
        expect(screen.getByText('25,000')).toBeTruthy();
        expect(screen.getByText('10,000')).toBeTruthy();
      });
    });
  });

  describe('Error Handling', () => {
    it('should display error message when API fails', async () => {
      getAccounts.mockRejectedValue(new Error('Network error occurred'));
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText('Accounts unavailable')).toBeTruthy();
        expect(screen.getByText('Network error occurred')).toBeTruthy();
      });
    });

    it('should display default error message when no message provided', async () => {
      getAccounts.mockRejectedValue({});
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText('Failed to load accounts.')).toBeTruthy();
      });
    });
  });

  describe('Search Functionality', () => {
    it('should update search input', async () => {
      const user = userEvent.setup();
      getAccounts.mockResolvedValue(mockAccounts);
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText(/@testuser1/)).toBeTruthy();
      });
      
      const searchInput = screen.getByPlaceholderText(/username, display name, external id/i);
      await user.type(searchInput, 'testuser');
      
      expect(searchInput.value).toBe('testuser');
    });
  });

  describe('Filter Functionality', () => {
    it('should toggle verified filter', async () => {
      const user = userEvent.setup();
      getAccounts.mockResolvedValue(mockAccounts);
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText(/@testuser1/)).toBeTruthy();
      });
      
      const verifiedCheckbox = screen.getByRole('checkbox', { name: /Verified/i });
      await user.click(verifiedCheckbox);
      
      expect(verifiedCheckbox.checked).toBe(true);
    });

    it('should toggle bots filter', async () => {
      const user = userEvent.setup();
      getAccounts.mockResolvedValue(mockAccounts);
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText(/@testuser1/)).toBeTruthy();
      });
      
      const botsCheckbox = screen.getByRole('checkbox', { name: /Bots/i });
      await user.click(botsCheckbox);
      
      expect(botsCheckbox.checked).toBe(true);
    });
  });

  describe('Pagination', () => {
    it('should display pagination info with multiple pages', async () => {
      getAccounts.mockResolvedValue({
        items: mockAccounts.items,
        total: 150,
      });
      render(<Accounts />);
      
      await waitFor(() => {
        const paginationTexts = screen.getAllByText(/Page 1 \/ 3/i);
        expect(paginationTexts.length).toBe(2);
        expect(paginationTexts[0]).toBeTruthy();
      });
    });
  });

  describe('Empty State', () => {
    it('should show empty message when no accounts', async () => {
      getAccounts.mockResolvedValue({
        items: [],
        total: 0,
      });
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText('No accounts match current filters.')).toBeTruthy();
      });
    });
  });

  describe('Metric Values', () => {
    it('should display manipulation and risk scores correctly', async () => {
      getAccounts.mockResolvedValue(mockAccounts);
      render(<Accounts />);
      
      await waitFor(() => {
        expect(screen.getByText('0.85')).toBeTruthy();
        expect(screen.getByText('0.65')).toBeTruthy();
        expect(screen.getByText('0.45')).toBeTruthy();
        expect(screen.getByText('0.35')).toBeTruthy();
      });
    });
  });
});

// Helper functions tests
describe('Helper Functions', () => {
  describe('scoreTone', () => {
    const scoreTone = (value) => {
      if ((value || 0) >= 0.7) return 'high';
      if ((value || 0) >= 0.4) return 'med';
      return 'low';
    };

    it('should return high for values >= 0.7', () => {
      expect(scoreTone(0.7)).toBe('high');
      expect(scoreTone(0.85)).toBe('high');
      expect(scoreTone(1.0)).toBe('high');
    });

    it('should return med for values between 0.4 and 0.7', () => {
      expect(scoreTone(0.4)).toBe('med');
      expect(scoreTone(0.55)).toBe('med');
      expect(scoreTone(0.69)).toBe('med');
    });

    it('should return low for values < 0.4', () => {
      expect(scoreTone(0)).toBe('low');
      expect(scoreTone(0.39)).toBe('low');
      expect(scoreTone(-0.1)).toBe('low');
    });

    it('should handle null/undefined', () => {
      expect(scoreTone(null)).toBe('low');
      expect(scoreTone(undefined)).toBe('low');
    });
  });

  describe('formatNumber', () => {
    const formatNumber = (value) => {
      return new Intl.NumberFormat('en-US').format(Math.round(Number(value || 0)));
    };

    it('should format numbers with commas', () => {
      expect(formatNumber(1000)).toBe('1,000');
      expect(formatNumber(1000000)).toBe('1,000,000');
      expect(formatNumber(1234567)).toBe('1,234,567');
    });

    it('should round numbers', () => {
      expect(formatNumber(1234.56)).toBe('1,235');
      expect(formatNumber(99.99)).toBe('100');
    });

    it('should handle zero', () => {
      expect(formatNumber(0)).toBe('0');
    });

    it('should handle null/undefined', () => {
      expect(formatNumber(null)).toBe('0');
      expect(formatNumber(undefined)).toBe('0');
    });
  });

  describe('formatDate', () => {
    const formatDate = (value) => {
      return value ? value.slice(0, 10) : '-';
    };

    it('should format ISO date to YYYY-MM-DD', () => {
      expect(formatDate('2024-01-15T10:30:00Z')).toBe('2024-01-15');
      expect(formatDate('2024-12-25T00:00:00Z')).toBe('2024-12-25');
    });

    it('should return dash for null/undefined', () => {
      expect(formatDate(null)).toBe('-');
      expect(formatDate(undefined)).toBe('-');
    });

    it('should handle empty string', () => {
      expect(formatDate('')).toBe('-');
    });
  });
});