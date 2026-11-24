import { act, fireEvent, render, screen } from '@testing-library/react';
import { SessionList } from './session-list';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('../hooks/use-select-derived-session-list', () => ({
  useSelectDerivedSessionList: () => ({
    sessions: [
      { id: 'session-1', name: 'Alpha' },
      { id: 'session-2', name: 'Beta' },
    ],
    total: 20,
    loading: false,
    loadingMore: false,
    handleScroll: jest.fn(),
    addTemporarySession: jest.fn(),
    removeTemporarySession: jest.fn(),
  }),
}));

jest.mock('./session-card', () => ({
  SessionCard: ({ session }: { session: { name: string } }) => (
    <div>{session.name}</div>
  ),
}));

const getSessionCount = () =>
  screen.getByText('explore.sessions').nextElementSibling;

const applySearch = async (value: string) => {
  fireEvent.change(screen.getByPlaceholderText('explore.searchSessions'), {
    target: { value },
  });
  await act(async () => {
    jest.advanceTimersByTime(300);
  });
};

describe('SessionList search count', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('follows the applied filter and keeps the backend total for blank input', async () => {
    render(<SessionList onSelectSession={jest.fn()} />);

    expect(getSessionCount()).toHaveTextContent('20');

    fireEvent.change(screen.getByPlaceholderText('explore.searchSessions'), {
      target: { value: ' Alpha ' },
    });
    expect(getSessionCount()).toHaveTextContent('20');

    await act(async () => {
      jest.advanceTimersByTime(300);
    });
    expect(getSessionCount()).toHaveTextContent('1');
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.queryByText('Beta')).not.toBeInTheDocument();

    await applySearch('   ');
    expect(getSessionCount()).toHaveTextContent('20');
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.getByText('Beta')).toBeInTheDocument();

    await applySearch('missing');
    expect(getSessionCount()).toHaveTextContent('0');
  });
});
