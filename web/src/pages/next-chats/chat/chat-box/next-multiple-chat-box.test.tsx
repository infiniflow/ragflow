import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { useCallback, useEffect } from 'react';
import { useFormContext } from 'react-hook-form';
import { MultipleChatBox } from './next-multiple-chat-box';

const mockUpdateChat = jest.fn();
const mockHandlePressEnter = jest.fn();
const mockFindLlmByUuid = jest.fn();
let mockModelFormValues: Record<string, unknown> = {};

function MockLargeModelFormFieldWithoutFilter() {
  const form = useFormContext();

  useEffect(() => {
    Object.entries(mockModelFormValues).forEach(([name, value]) => {
      form.setValue(name, value);
    });
  }, [form]);

  return null;
}

type MockNextMessageInputProps = {
  onPressEnter(params: {
    enableInternet: boolean;
    enableThinking: boolean;
  }): void;
};

function MockNextMessageInput({ onPressEnter }: MockNextMessageInputProps) {
  const handleClick = useCallback(() => {
    onPressEnter({ enableInternet: false, enableThinking: false });
  }, [onPressEnter]);

  return <button data-testid="mock-send-message" onClick={handleClick} />;
}

jest.mock('@/components/large-model-form-field', () => ({
  LargeModelFormFieldWithoutFilter: () =>
    MockLargeModelFormFieldWithoutFilter(),
}));
jest.mock('@/components/message-input/next', () => ({
  NextMessageInput: (props: MockNextMessageInputProps) =>
    MockNextMessageInput(props),
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
jest.mock('@/hooks/logic-hooks', () => ({
  useHandleMessageInputChange: () => ({
    handleInputChange: jest.fn(),
    value: 'Question',
    setValue: jest.fn(),
  }),
  useScrollToBottom: () => ({ scrollRef: { current: null } }),
}));
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchChat: () => ({
    data: {
      icon: '',
      llm_id: 'previous-model',
      llm_setting: {
        frequency_penalty: 0.8,
        max_tokens: 8192,
      },
      tenant_id: 'tenant-1',
    },
  }),
  useGetChatSearchParams: () => ({ conversationId: 'conversation-1' }),
  useUpdateChat: () => ({ updateChat: mockUpdateChat }),
}));
jest.mock('@/hooks/use-llm-request', () => ({
  useFindLlmByUuid: () => mockFindLlmByUuid,
}));
jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({ data: { nickname: '', avatar: '' } }),
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
jest.mock('../../hooks/use-send-single-message', () => ({
  useSendSingleMessage: () => ({
    derivedMessages: [],
    handlePressEnter: mockHandlePressEnter,
    sendLoading: false,
  }),
}));
jest.mock('../../hooks/use-upload-file', () => ({
  useUploadFile: () => ({
    handleUploadFile: jest.fn(),
    isUploading: false,
    files: [],
    clearFiles: jest.fn(),
    removeFile: jest.fn(),
  }),
}));
jest.mock('../../hooks/use-message-references', () => ({
  useMessageReferences: () => new Map(),
}));
jest.mock('../../utils', () => ({ EmptyReference: {} }));
jest.mock('../use-show-internet', () => ({
  useShowInternet: () => false,
}));
jest.mock('react-router', () => ({
  useParams: () => ({ id: 'dialog-1' }),
}));

function renderMultipleChatBox() {
  return render(
    <TooltipProvider>
      <MultipleChatBox
        controller={new AbortController()}
        chatBoxIds={['card-1']}
        removeChatBox={jest.fn()}
        addChatBox={jest.fn()}
        stopOutputMessage={jest.fn()}
        conversation={{} as never}
      />
    </TooltipProvider>,
  );
}

describe('MultipleChatBox model settings', () => {
  beforeEach(() => {
    mockUpdateChat.mockReset();
    mockHandlePressEnter.mockReset();
    mockFindLlmByUuid.mockReset();
    mockFindLlmByUuid.mockReturnValue({ model_type: 'chat' });
    mockModelFormValues = {
      llm_id: 'model-1',
      parameter: 'Custom',
      temperature: 0.2,
      top_p: 0.75,
      presence_penalty: 0.5,
      frequency_penalty: 0.5,
      max_tokens: 4096,
      temperatureEnabled: true,
      topPEnabled: true,
      presencePenaltyEnabled: true,
      frequencyPenaltyEnabled: false,
      maxTokensEnabled: false,
      thinking: 'default',
    };
  });

  it('replaces the saved settings without disabled parameters or UI flags', async () => {
    renderMultipleChatBox();

    const applyButton = screen.getByTestId('chat-detail-multimodel-card-apply');
    await waitFor(() => expect(applyButton).toBeEnabled());
    fireEvent.click(applyButton);

    expect(mockUpdateChat).toHaveBeenCalledWith({
      chatId: 'dialog-1',
      params: {
        llm_id: 'model-1',
        tenant_llm_id: 'model-1',
        llm_setting: {
          temperature: 0.2,
          top_p: 0.75,
          presence_penalty: 0.5,
          thinking: 'default',
          model_type: 'chat',
        },
      },
    });
  });

  it('keeps an enabled preset frequency penalty but omits disabled max tokens', async () => {
    mockModelFormValues.parameter = 'Precise';
    mockModelFormValues.frequencyPenaltyEnabled = true;
    renderMultipleChatBox();

    const applyButton = screen.getByTestId('chat-detail-multimodel-card-apply');
    await waitFor(() => expect(applyButton).toBeEnabled());
    fireEvent.click(applyButton);

    const request = mockUpdateChat.mock.calls[0][0];
    expect(request.params.llm_setting).toMatchObject({
      frequency_penalty: 0.5,
    });
    expect(request.params.llm_setting).not.toHaveProperty('max_tokens');
    expect(request.params.llm_setting).not.toHaveProperty(
      'frequencyPenaltyEnabled',
    );
    expect(request.params.llm_setting).not.toHaveProperty('maxTokensEnabled');
    expect(request.params.llm_setting).not.toHaveProperty('parameter');
  });

  it('omits disabled parameters and UI flags from multi-model messages', () => {
    renderMultipleChatBox();

    fireEvent.click(screen.getByTestId('mock-send-message'));

    expect(mockHandlePressEnter).toHaveBeenCalledWith({
      enableInternet: false,
      enableThinking: false,
      llm_id: 'model-1',
      temperature: 0.2,
      top_p: 0.75,
      presence_penalty: 0.5,
      thinking: 'default',
      storeHistoryMessages: false,
      omitSessionId: true,
    });
  });
});
