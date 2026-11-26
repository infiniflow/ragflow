import { act, fireEvent, render, screen } from '@testing-library/react';
import { SessionList } from './session-list';

const mockAddTemporarySession = jest.fn();

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
    addTemporarySession: mockAddTemporarySession,
    removeTemporarySession: jest.fn(),
  }),
}));

jest.mock('./session-dropdown', () => ({
  SessionDropdown: ({ children }: { children: React.ReactNode }) => children,
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

describe('SessionList', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockAddTemporarySession.mockClear();
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

  it('creates a temporary session from the new conversation button', () => {
    render(<SessionList onSelectSession={jest.fn()} />);

    fireEvent.click(screen.getByTestId('agent-explore-session-new'));

    expect(mockAddTemporarySession).toHaveBeenCalledTimes(1);
  });

  it('selects a session through the shared conversation list', () => {
    const onSelectSession = jest.fn();
    render(<SessionList onSelectSession={onSelectSession} />);

    fireEvent.click(screen.getByRole('button', { name: 'Alpha' }));

    expect(onSelectSession).toHaveBeenCalledWith('session-1', false);
  });
});
