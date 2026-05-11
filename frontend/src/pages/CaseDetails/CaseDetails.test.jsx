import { render, screen, waitFor } from '@testing-library/react';
import { CaseDetails } from './CaseDetails';
import { getCaseDetails, getCaseScores } from '../../services/api';

jest.mock('../../services/api');
jest.mock('react-router-dom', () => ({
  useNavigate: () => jest.fn(),
  useParams: () => ({ id: '123' }),
}));

// Остальные моки как у вас были
jest.mock('../../components/ui', () => ({
  StateMessage: ({ title }) => <div>{title}</div>,
  PanelTitle: () => null,
  Breakdown: () => null,
  // ... остальные моки
}));

// Моки остальных зависимостей...

describe('CaseDetails', () => {
  it('renders loading', () => {
    getCaseDetails.mockImplementation(() => new Promise(() => {}));
    render(<CaseDetails />);
    expect(screen.getByText(/Loading case/i)).toBeTruthy();
  });

  it('renders error', async () => {
    getCaseDetails.mockRejectedValue(new Error('Error'));
    render(<CaseDetails />);
    await waitFor(() => {
      expect(screen.getByText('Case unavailable')).toBeTruthy();
    });
  });
});