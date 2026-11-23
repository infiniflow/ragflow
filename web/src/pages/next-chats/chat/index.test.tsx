import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import Chat from './index';

const mockAddTemporaryConversation = jest.fn();
const mockRemoveTemporaryConversation = jest.fn();
const mockHandleConversationCardClick = jest.fn();
const mockSetConversationBoth = jest.fn();
const mockRemoveSessions = jest.fn();
const mockRemoveStreamSessions = jest.fn();
const mockFetchSessionManually = jest.fn();
const mockShowEmbedModal = jest.fn();
const mockHideEmbedModal = jest.fn();
const mockSetChatSidebarCollapsed = jest.fn();
const mockSetPrimarySidebarCollapsed = jest.fn();
let mockChatSidebarCollapsed = false;

function MockRootLayout({ children }: { children: React.ReactNode }) {
  return <div>{children}</div>;
}

function MockSingleChatBox() {
  return <div data-testid="mock-single-chat-box" />;
}

function MockMultipleChatBox() {
  return <div data-testid="mock-multiple-chat-box" />;
}

function MockChatSettings() {
  return <section data-testid="chat-detail-settings" />;
}

function MockConversationDropdown({ children }: { children: React.ReactNode }) {
  return children;
}

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('react-router', () => ({
  useParams: () => ({ id: 'chat-1' }),
}));
jest.mock('@/hooks/use-chat-sidebar-collapse', () => ({
  useChatSidebarCollapse: () => ({
    collapsed: mockChatSidebarCollapsed,
    setCollapsed: mockSetChatSidebarCollapsed,
  }),
}));
jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));
jest.mock('@/layouts/root-layout', () => ({
  RootLayoutContainer: MockRootLayout,
}));
jest.mock('@/components/embed-dialog', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('@/components/embed-dialog/use-show-embed-dialog', () => ({
  useShowEmbedModal: () => ({
    showEmbedModal: mockShowEmbedModal,
    hideEmbedModal: mockHideEmbedModal,
    embedVisible: false,
    beta: '',
  }),
}));
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchSessionManually: () => ({
    fetchSessionManually: mockFetchSessionManually,
  }),
  useGetChatSearchParams: () => ({ conversationId: '', isNew: '' }),
  useFetchChat: () => ({ data: { name: 'Chat app', icon: '' } }),
  useRemoveSessions: () => ({ removeSessions: mockRemoveSessions }),
}));
jest.mock('../hooks/use-click-card', () => ({
  useHandleClickConversationCard: () => ({
    handleConversationCardClick: mockHandleConversationCardClick,
    controller: new AbortController(),
    stopOutputMessage: jest.fn(),
  }),
}));
jest.mock('../hooks/use-chat-url', () => ({
  useChatUrlParams: () => ({ setConversationBoth: mockSetConversationBoth }),
}));
jest.mock('../hooks/use-select-conversation-list', () => ({
  useSelectDerivedConversationList: () => ({
    list: [
      {
        id: 'conversation-1',
        name: 'Conversation one',
        chat_id: 'chat-1',
        is_new: false,
      },
    ],
    total: 1,
    addTemporaryConversation: mockAddTemporaryConversation,
    removeTemporaryConversation: mockRemoveTemporaryConversation,
    handleInputChange: jest.fn(),
    searchString: '',
    loadingMore: false,
    handleScroll: jest.fn(),
  }),
}));
jest.mock('../chat-stream/store', () => ({
  useChatStreamStore: () => mockRemoveStreamSessions,
}));
jest.mock('./app-settings/chat-settings', () => ({
  ChatSettings: MockChatSettings,
}));
jest.mock('./chat-box/single-chat-box', () => ({
  SingleChatBox: MockSingleChatBox,
}));
jest.mock('./chat-box/next-multiple-chat-box', () => ({
  MultipleChatBox: MockMultipleChatBox,
}));
jest.mock('./conversation-dropdown', () => ({
  ConversationDropdown: MockConversationDropdown,
}));

function renderChat() {
  return render(
    <TooltipProvider>
      <Chat />
    </TooltipProvider>,
  );
}

describe('Chat layout navigation', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockChatSidebarCollapsed = false;
  });

  it('opens settings as the full right-hand view and exits through session navigation', () => {
    renderChat();

    expect(screen.getByText('Chat app')).toBeInTheDocument();
    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();
    const embedButton = screen.getByTestId('chat-detail-embed-open');
    const contentFrame = screen.getByTestId('chat-detail-content-frame');

    fireEvent.click(screen.getByTestId('chat-settings'));

    expect(screen.getByTestId('chat-settings')).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByTestId('chat-detail-sessions')).toBeInTheDocument();
    expect(screen.getByTestId('chat-detail-settings')).toBeInTheDocument();
    expect(
      screen.queryByTestId('mock-single-chat-box'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('chat-detail-embed-open')).toBe(embedButton);
    expect(screen.getByTestId('chat-detail-content-frame')).toBe(contentFrame);

    fireEvent.click(screen.getByTestId('chat-detail-session-new'));
    expect(screen.getByTestId('chat-settings')).toHaveAttribute(
      'aria-pressed',
      'false',
    );
    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();
    expect(
      screen.queryByTestId('chat-detail-settings'),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('chat-settings'));
    fireEvent.click(screen.getByTestId('chat-detail-session-item'));
    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();
    expect(
      screen.queryByTestId('chat-detail-settings'),
    ).not.toBeInTheDocument();
  });

  it('keeps the shared header and frame mounted in multiple models', () => {
    renderChat();

    const embedButton = screen.getByTestId('chat-detail-embed-open');
    const contentFrame = screen.getByTestId('chat-detail-content-frame');
    fireEvent.click(screen.getByTestId('chat-detail-multimodel-toggle'));

    expect(screen.getByTestId('chat-detail-sessions')).toBeInTheDocument();
    expect(screen.getByTestId('mock-multiple-chat-box')).toBeInTheDocument();
    expect(
      screen.queryByTestId('mock-single-chat-box'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('chat-detail-embed-open')).toBe(embedButton);
    expect(screen.getByTestId('chat-detail-content-frame')).toBe(contentFrame);

    fireEvent.click(screen.getByTestId('chat-detail-session-item'));

    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();
    expect(
      screen.queryByTestId('mock-multiple-chat-box'),
    ).not.toBeInTheDocument();
  });

  it('returns to a single conversation when creating or selecting a session', () => {
    renderChat();

    fireEvent.click(screen.getByTestId('chat-detail-multimodel-toggle'));
    fireEvent.click(screen.getByTestId('chat-detail-session-new'));

    expect(mockAddTemporaryConversation).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('chat-detail-multimodel-toggle'));
    fireEvent.click(screen.getByTestId('chat-detail-session-item'));

    expect(mockHandleConversationCardClick).toHaveBeenCalledWith(
      'conversation-1',
      false,
    );
    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();
  });

  it('replaces multiple models with settings without replacing the shared layout', () => {
    renderChat();

    const embedButton = screen.getByTestId('chat-detail-embed-open');
    const contentFrame = screen.getByTestId('chat-detail-content-frame');
    fireEvent.click(screen.getByTestId('chat-detail-multimodel-toggle'));
    fireEvent.click(screen.getByTestId('chat-settings'));

    expect(
      screen.queryByTestId('mock-single-chat-box'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('chat-detail-settings')).toBeInTheDocument();
    expect(screen.getByTestId('chat-detail-embed-open')).toBe(embedButton);
    expect(screen.getByTestId('chat-detail-content-frame')).toBe(contentFrame);
    expect(
      screen.queryByTestId('mock-multiple-chat-box'),
    ).not.toBeInTheDocument();
  });

  it('uses the existing Embed into webpage dialog entry', () => {
    renderChat();

    expect(screen.getByText('common.embedIntoSite')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('chat-detail-embed-open'));

    expect(mockShowEmbedModal).toHaveBeenCalledTimes(1);
  });

  it('collapses both sidebars from the secondary sidebar header', () => {
    renderChat();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(mockSetChatSidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
  });

  it('hides the secondary sidebar without unmounting the chat content', () => {
    mockChatSidebarCollapsed = true;
    renderChat();

    expect(
      screen.queryByTestId('chat-detail-sessions'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('mock-single-chat-box')).toBeInTheDocument();
  });
});
