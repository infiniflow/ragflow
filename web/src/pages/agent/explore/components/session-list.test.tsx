import { useFetchSessionsByCanvasId } from '@/hooks/use-agent-request';
import { useClientSearch } from '@/hooks/use-client-search';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useExploreUrlParams } from '../hooks/use-explore-url-params';
import { SessionList } from './session-list';

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchSessionsByCanvasId: jest.fn(),
}));

jest.mock('@/hooks/use-client-search', () => ({
  useClientSearch: jest.fn(),
}));

jest.mock('../hooks/use-explore-url-params', () => ({
  useExploreUrlParams: jest.fn(),
}));

jest.mock('./session-card', () => ({
  SessionCard: ({
    onClick,
    session,
  }: {
    onClick: () => void;
    session: { is_new?: boolean };
  }) => (
    <button data-testid="session-card" onClick={onClick}>
      {session.is_new ? 'New Session' : 'Existing Session'}
    </button>
  ),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const mockUseFetchSessionsByCanvasId = useFetchSessionsByCanvasId as jest.Mock;
const mockUseClientSearch = useClientSearch as jest.Mock;
const mockUseExploreUrlParams = useExploreUrlParams as jest.Mock;

describe('SessionList temporary session behavior', () => {
  it('marks and selects the temporary session with an empty id', async () => {
    const setSessionId = jest.fn();
    const sessions: never[] = [];

    mockUseFetchSessionsByCanvasId.mockReturnValue({
      data: sessions,
      loading: false,
    });
    mockUseClientSearch.mockImplementation(({ data }: { data: unknown[] }) => ({
      filteredData: data,
      handleSearchChange: jest.fn(),
      searchKeyword: '',
    }));
    mockUseExploreUrlParams.mockReturnValue({ setSessionId });

    const onSelectSession = jest.fn();
    render(<SessionList onSelectSession={onSelectSession} />);

    fireEvent.click(screen.getAllByRole('button')[0]);

    expect(setSessionId).toHaveBeenCalledWith('', true);
    const temporaryCard = await waitFor(() =>
      screen.getByTestId('session-card'),
    );
    expect(temporaryCard).toHaveTextContent('New Session');

    fireEvent.click(temporaryCard);
    expect(onSelectSession).toHaveBeenCalledWith('', true);
  });
});
