import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { HelpMenu } from './help-menu';

const mockFetch = jest.fn();
const mockOpen = jest.fn();
const mockFetchSystemVersion = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchSystemVersion: () => ({
    version: 'v0.15.1',
    fetchSystemVersion: mockFetchSystemVersion,
  }),
}));

jest.mock('@/components/icon-font', () => ({
  IconFontFill: ({ name }: { name: string }) => (
    <span aria-hidden="true">{name}</span>
  ),
}));

function renderHelpMenu() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <HelpMenu />
    </QueryClientProvider>,
  );
}

function openHelpMenu() {
  fireEvent.keyDown(screen.getByRole('button', { name: 'header.help' }), {
    key: 'ArrowDown',
  });
}

describe('HelpMenu', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    Object.defineProperty(globalThis, 'fetch', {
      configurable: true,
      writable: true,
      value: mockFetch,
    });
    Object.defineProperty(window, 'open', {
      configurable: true,
      writable: true,
      value: mockOpen,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: jest.fn().mockResolvedValue({ stargazers_count: 97280 }),
    });
  });

  it('loads and formats GitHub stars only after the menu opens', async () => {
    renderHelpMenu();

    expect(mockFetch).not.toHaveBeenCalled();
    expect(mockFetchSystemVersion).toHaveBeenCalledTimes(1);

    openHelpMenu();

    expect(await screen.findByText('97,280')).toBeInTheDocument();
    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch).toHaveBeenCalledWith(
      'https://api.github.com/repos/infiniflow/ragflow',
    );
    expect(screen.getByText('sidebar.version')).toBeInTheDocument();
    expect(screen.getByText('v0.15.1')).toBeInTheDocument();
  });

  it('opens Docs, Discord, and GitHub in new tabs', async () => {
    renderHelpMenu();

    openHelpMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: 'sidebar.docs' }));
    expect(mockOpen).toHaveBeenLastCalledWith(
      'https://ragflow.io/docs/dev/category/user-guides',
      '_blank',
      'noreferrer,noopener',
    );

    openHelpMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: 'header.discord' }));
    expect(mockOpen).toHaveBeenLastCalledWith(
      'https://discord.com/invite/NjYzJD3GM3',
      '_blank',
      'noreferrer,noopener',
    );

    openHelpMenu();
    fireEvent.click(
      await screen.findByRole('menuitem', { name: /header\.github/ }),
    );
    expect(mockOpen).toHaveBeenLastCalledWith(
      'https://github.com/infiniflow/ragflow',
      '_blank',
      'noreferrer,noopener',
    );
  });

  it('keeps the GitHub link usable when the star request fails', async () => {
    mockFetch.mockResolvedValue({ ok: false, status: 403 });
    renderHelpMenu();

    openHelpMenu();

    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
    expect(
      screen.getByRole('menuitem', { name: 'header.github' }),
    ).toBeInTheDocument();
    expect(screen.queryByText('97,280')).not.toBeInTheDocument();
  });
});
