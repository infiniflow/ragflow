import { render, screen } from '@testing-library/react';

import { ChatList } from '../chat-list';

jest.mock('@/hooks/use-chat-request', () => ({
  useFetchChatList: () => ({ data: undefined, loading: false }),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToChat: () => () => {} }),
}));

jest.mock('../../next-chats/hooks/use-rename-chat', () => ({
  useRenameChat: () => ({
    initialChatName: '',
    chatRenameVisible: false,
    showChatRenameModal: jest.fn(),
    hideChatRenameModal: jest.fn(),
    onChatRenameOk: jest.fn(),
    chatRenameLoading: false,
  }),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('ChatList', () => {
  it('renders without crashing when chat list data is undefined', () => {
    const setListLength = jest.fn();
    const { container } = render(
      <ChatList setListLength={setListLength} setLoading={jest.fn()} />,
    );

    expect(container).toBeTruthy();
    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument();
    expect(setListLength).toHaveBeenCalledWith(0);
  });
});
