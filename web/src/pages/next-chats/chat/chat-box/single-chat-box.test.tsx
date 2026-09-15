import { render, waitFor } from '@testing-library/react';
import React from 'react';

import { MessageType } from '@/constants/chat';
import { SingleChatBox } from './single-chat-box';

const mockHydrateFromServer = jest.fn();
const mockUseGetChatSearchParams = jest.fn();

jest.mock('@/components/message-input/next', () => ({
  NextMessageInput: () => null,
}));
jest.mock('@/components/message-item', () => () => null);
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
    messages: [],
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

function buildConversation(id: string) {
  return {
    id,
    chat_id: 'chat-id',
    name: 'Conversation',
    avatar: '',
    create_date: '',
    create_time: 0,
    update_date: '',
    update_time: 0,
    is_new: true as const,
    reference: [],
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
});
