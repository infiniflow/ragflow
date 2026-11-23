import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import SearchPage from './index';

const mockShowEmbedModal = jest.fn();
const mockHideEmbedModal = jest.fn();
const mockSetSearchSidebarCollapsed = jest.fn();
const mockSetPrimarySidebarCollapsed = jest.fn();
let mockSearchSidebarCollapsed = false;
let mockSettingsRequired = false;

function MockSearchHome() {
  return <div data-testid="mock-search-home" />;
}

function MockSearchingPage() {
  return <div data-testid="mock-searching-page" />;
}

function MockSearchSetting({ onClose }: { onClose: () => void }) {
  return (
    <section data-testid="search-detail-settings">
      <button type="button" onClick={onClose}>
        Close settings
      </button>
    </section>
  );
}

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-search-sidebar-collapse', () => ({
  useSearchSidebarCollapse: () => ({
    collapsed: mockSearchSidebarCollapsed,
    setCollapsed: mockSetSearchSidebarCollapsed,
  }),
}));
jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));
jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({ data: { nickname: 'KiKi' } }),
}));
jest.mock('../next-searches/hooks', () => ({
  useFetchSearchDetail: () => ({
    data: { id: 'search-1', name: 'Search demo kiki' },
  }),
}));
jest.mock('./hooks', () => ({
  useCheckSettings: () => ({ openSetting: mockSettingsRequired }),
}));
jest.mock('@/components/embed-dialog/use-show-embed-dialog', () => ({
  useShowEmbedModal: () => ({
    showEmbedModal: mockShowEmbedModal,
    hideEmbedModal: mockHideEmbedModal,
    embedVisible: false,
    beta: '',
  }),
}));
jest.mock('./embed-app-modal', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('./search-home', () => ({
  __esModule: true,
  default: MockSearchHome,
}));
jest.mock('./searching', () => ({
  __esModule: true,
  default: MockSearchingPage,
}));
jest.mock('./search-setting', () => ({
  SearchSetting: MockSearchSetting,
}));

function renderSearchPage() {
  return render(
    <TooltipProvider>
      <SearchPage />
    </TooltipProvider>,
  );
}

describe('Search detail layout', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSearchSidebarCollapsed = false;
    mockSettingsRequired = false;
  });

  it('switches between search and settings without replacing the shared chrome', () => {
    renderSearchPage();

    expect(screen.getByText('Search demo kiki')).toBeInTheDocument();
    expect(screen.getByTestId('mock-search-home')).toBeInTheDocument();
    const embedButton = screen.getByTestId('search-detail-embed-open');
    const contentFrame = screen.getByTestId('search-detail-content-frame');

    fireEvent.click(screen.getByTestId('search-menu-settings'));

    expect(screen.getByTestId('search-menu-settings')).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByTestId('search-detail-settings')).toBeInTheDocument();
    expect(screen.queryByTestId('mock-search-home')).not.toBeInTheDocument();
    expect(screen.getByTestId('search-detail-embed-open')).toBe(embedButton);
    expect(screen.getByTestId('search-detail-content-frame')).toBe(
      contentFrame,
    );
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);

    fireEvent.click(screen.getByTestId('search-menu-search'));

    expect(screen.getByTestId('mock-search-home')).toBeInTheDocument();
    expect(
      screen.queryByTestId('search-detail-settings'),
    ).not.toBeInTheDocument();
  });

  it('opens settings automatically when configuration is incomplete', () => {
    mockSettingsRequired = true;
    renderSearchPage();

    expect(screen.getByTestId('search-detail-settings')).toBeInTheDocument();
    expect(screen.queryByTestId('mock-search-home')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('search-menu-search'));
    expect(screen.getByTestId('mock-search-home')).toBeInTheDocument();
  });

  it('uses the existing Embed into webpage entry', () => {
    renderSearchPage();

    expect(screen.getByText('common.embedIntoSite')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('search-detail-embed-open'));

    expect(mockShowEmbedModal).toHaveBeenCalledTimes(1);
  });

  it('collapses both sidebars from the secondary sidebar header', () => {
    renderSearchPage();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(mockSetSearchSidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
  });

  it('keeps search content mounted when the secondary sidebar is collapsed', () => {
    mockSearchSidebarCollapsed = true;
    renderSearchPage();

    expect(
      screen.queryByTestId('search-detail-sidebar'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('mock-search-home')).toBeInTheDocument();
  });
});
