import { render, waitFor } from '@testing-library/react';

import { ChatSessionSource, MessageType } from '@/constants/chat';
import { SingleChatBox } from './single-chat-box';

const mockHydrateFromServer = jest.fn();
const mockUseGetChatSearchParams = jest.fn();
const mockNextMessageInput = jest.fn<null, [props: Record<string, unknown>]>(
  () => null,
);
const mockMessageItem = jest.fn<null, [props: Record<string, unknown>]>(
  () => null,
);
const mockMessages = [
  { id: 'message-id', role: MessageType.User, content: 'Message' },
];

jest.mock('@/components/message-input/next', () => ({
  NextMessageInput: (props: Record<string, unknown>) =>
    mockNextMessageInput(props),
}));
jest.mock(
  '@/components/message-item',
  () => (props: Record<string, unknown>) => mockMessageItem(props),
);
jest.mock('@/components/virtual-message-list', () => ({
  VirtualMessageList: ({
    items,
    renderItem,
  }: {
    items: unknown[];
    renderItem: (item: unknown, index: number) => null;
  }) =>
    items.map((item, index) => (
      <div key={index}>{renderItem(item, index)}</div>
    )),
}));
jest.mock('@/components/pdf-drawer', () => () => null);
jest.mock('@/components/pdf-drawer/hooks', () => ({
  useClickDrawer: () => ({
    visible: false,
    hideModal: jest.fn(),
    documentId: '',
    selectedChunk: undefined,
    clickDocumentButton: jest.fn(),
  }),
}));
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchChat: () => ({ data: { icon: '' } }),
  useGetChatSearchParams: () => mockUseGetChatSearchParams(),
}));
jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({ data: { nickname: '', avatar: '' } }),
}));
jest.mock('@/utils/chat', () => ({
  buildMessageUuidWithRole: () => 'message-key',
}));
jest.mock('../../hooks/use-button-disabled', () => ({
  useGetSendButtonDisabled: () => false,
  useSendButtonDisabled: () => false,
}));
jest.mock('../../hooks/use-create-conversation', () => ({
  useCreateConversationBeforeUploadDocument: () => ({
    createConversationBeforeUploadDocument: jest.fn(),
  }),
}));
jest.mock('../../hooks/use-send-chat-message', () => ({
  useSendMessage: () => ({
    value: '',
    scrollRef: { current: null },
    messageContainerRef: { current: null },
    sendLoading: false,
    messages: mockMessages,
    isUploading: false,
    handleInputChange: jest.fn(),
    handlePressEnter: jest.fn(),
    regenerateMessage: jest.fn(),
    removeMessageById: jest.fn(),
    handleUploadFile: jest.fn(),
    removeFile: jest.fn(),
    hydrateFromServer: mockHydrateFromServer,
    stopOutputMessage: jest.fn(),
  }),
}));
jest.mock('../../utils', () => ({
  buildMessageItemReference: () => undefined,
}));
jest.mock('../use-show-internet', () => ({
  useShowInternet: () => false,
}));

function buildConversation(id: string, source = ChatSessionSource.Normal) {
  return {
    id,
    chat_id: 'chat-id',
    name: 'Conversation',
    source,
    avatar: '',
    create_date: '',
    create_time: 0,
    update_date: '',
    update_time: 0,
    is_new: true as const,
    messages: [
      {
        id: 'old-message',
        role: MessageType.User,
        content: 'Old conversation message',
      },
    ],
  };
}

describe('SingleChatBox conversation hydration', () => {
  beforeEach(() => {
    mockHydrateFromServer.mockClear();
    mockUseGetChatSearchParams.mockReset();
    mockNextMessageInput.mockClear();
    mockMessageItem.mockClear();
  });

  it('ignores stale messages while the URL points to a new conversation', () => {
    mockUseGetChatSearchParams.mockReturnValue({
      conversationId: 'new-conversation',
    });

    render(
      <SingleChatBox conversation={buildConversation('old-conversation')} />,
    );

    expect(mockHydrateFromServer).not.toHaveBeenCalled();
  });

  it('hydrates messages when the conversation matches the URL', async () => {
    const conversation = buildConversation('current-conversation');
    mockUseGetChatSearchParams.mockReturnValue({
      conversationId: conversation.id,
    });

    render(<SingleChatBox conversation={conversation} />);

    await waitFor(() => {
      expect(mockHydrateFromServer).toHaveBeenCalledWith(
        conversation.id,
        conversation.messages,
      );
    });
  });

  it.each([
    { source: ChatSessionSource.Normal, expectedDisabled: false },
    { source: ChatSessionSource.ChatChannel, expectedDisabled: true },
  ])(
    'sets the message input disabled state for $source conversations',
    ({ source, expectedDisabled }) => {
      const conversation = buildConversation('current-conversation', source);
      mockUseGetChatSearchParams.mockReturnValue({
        conversationId: conversation.id,
      });

      render(<SingleChatBox conversation={conversation} />);

      const inputProps = mockNextMessageInput.mock.calls.at(-1)?.[0];
      expect(inputProps).toMatchObject({
        disabled: expectedDisabled,
        sendDisabled: expectedDisabled,
      });
    },
  );

  it.each([
    { source: ChatSessionSource.Normal, expectedActions: true },
    { source: ChatSessionSource.ChatChannel, expectedActions: false },
  ])(
    'sets message actions for $source conversations',
    ({ source, expectedActions }) => {
      const conversation = buildConversation('current-conversation', source);
      mockUseGetChatSearchParams.mockReturnValue({
        conversationId: conversation.id,
      });

      render(<SingleChatBox conversation={conversation} />);

      const messageItemProps = mockMessageItem.mock.calls.at(-1)?.[0];
      const expectedAction = expectedActions ? expect.any(Function) : undefined;
      expect(messageItemProps).toMatchObject({
        regenerateMessage: expectedAction,
        removeMessageById: expectedAction,
      });
    },
  );
});
