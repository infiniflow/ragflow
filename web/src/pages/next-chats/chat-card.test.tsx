import { fireEvent, render, screen } from '@testing-library/react';
import { ChatCard } from './chat-card';

const mockSetPrimarySidebarCollapsed = jest.fn();
const mockNavigateToChat = jest.fn();

function MockHomeCard({ onClick }: { onClick: () => void }) {
  return (
    <button data-testid="chat-card" onClick={onClick}>
      Chat card
    </button>
  );
}

function MockChatDropdown({ children }: { children: React.ReactNode }) {
  return children;
}

jest.mock('@/components/home-card', () => ({
  HomeCard: MockHomeCard,
}));
jest.mock('@/components/more-button', () => ({
  MoreButton: () => null,
}));
jest.mock('@/components/shared-badge', () => ({
  SharedBadge: () => null,
}));
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToChat: (id: string) => () => mockNavigateToChat(id),
  }),
}));
jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));
jest.mock('./chat-dropdown', () => ({
  ChatDropdown: MockChatDropdown,
}));

describe('ChatCard navigation', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('collapses the primary sidebar before navigating to chat detail', () => {
    render(
      <ChatCard
        data={{ id: 'chat-1' } as never}
        showChatRenameModal={jest.fn()}
        showPrivilegeModal={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId('chat-card'));

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockNavigateToChat).toHaveBeenCalledWith('chat-1');
    expect(
      mockSetPrimarySidebarCollapsed.mock.invocationCallOrder[0],
    ).toBeLessThan(mockNavigateToChat.mock.invocationCallOrder[0]);
  });
});
