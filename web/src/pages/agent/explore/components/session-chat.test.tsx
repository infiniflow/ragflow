import { render } from '@testing-library/react';
import { AgentSessionSource } from '@/constants/agent';
import { SessionChat } from './session-chat';

// Keep imported bindings out of type positions in this jest.mock file.
const mockNextMessageInput = jest.fn<null, [props: Record<string, unknown>]>(
  () => null,
);
const mockUseExploreUrlParams = jest.fn();
const mockShouldShowParameterDialog = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/message-input/next', () => ({
  NextMessageInput: (props: Record<string, unknown>) =>
    mockNextMessageInput(props),
}));
jest.mock('@/components/next-markdown-content', () => () => null);
jest.mock('@/components/next-message-item', () => () => null);
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
jest.mock('@/components/virtual-message-list', () => ({
  VirtualMessageList: () => null,
}));
jest.mock('@/hooks/use-agent-request', () => ({
  useUploadAgentFileWithProgress: () => ({
    uploadAgentFile: jest.fn(),
    loading: false,
  }),
}));
jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({ data: { nickname: '', avatar: '' } }),
}));
jest.mock('@/pages/agent/debug-content', () => () => null);
jest.mock('@/pages/agent/hooks/use-chat-logic', () => ({
  useAwaitComponentData: () => ({
    buildInputList: () => [],
    handleOk: jest.fn(),
    isWaiting: false,
  }),
}));
jest.mock('@/pages/agent/share/parameter-dialog', () => ({
  ParameterDialog: () => null,
}));
jest.mock('../hooks/use-explore-url-params', () => ({
  useExploreUrlParams: () => mockUseExploreUrlParams(),
}));
jest.mock('../hooks/use-send-session-message', () => ({
  useSendSessionMessage: () => ({
    value: 'Hello',
    derivedMessages: [],
    virtualizer: {},
    messageContainerRef: { current: null },
    sendLoading: false,
    handleInputChange: jest.fn(),
    handlePressEnter: jest.fn(),
    stopOutputMessage: jest.fn(),
    sendFormMessage: jest.fn(),
    canvasInfo: {},
    findReferenceByMessageId: jest.fn(),
    appendUploadResponseList: jest.fn(),
    removeFile: jest.fn(),
    parameterDialogVisible: false,
    handleParametersOk: jest.fn(),
    beginInputs: [],
    shouldShowParameterDialog: mockShouldShowParameterDialog,
    setDerivedMessages: jest.fn(),
    streamSessionId: undefined,
    requestedSessionId: undefined,
    reapplyStreamedAnswer: jest.fn(),
  }),
}));

function renderSessionChat(options: {
  sessionId?: string;
  isNew?: boolean;
  session?: { id: string; source: string | null; message: unknown[] };
}) {
  const {
    sessionId = 'session-1',
    isNew = false,
    session = {
      id: sessionId,
      source: AgentSessionSource.Agent,
      message: [],
    },
  } = options;
  mockUseExploreUrlParams.mockReturnValue({ sessionId, isNew });

  render(<SessionChat session={session as never} />);

  return mockNextMessageInput.mock.calls.at(-1)?.[0];
}

describe('SessionChat message input source', () => {
  beforeEach(() => {
    mockNextMessageInput.mockClear();
    mockUseExploreUrlParams.mockReset();
    mockShouldShowParameterDialog.mockClear();
  });

  it.each([
    { source: AgentSessionSource.Agent, expectedDisabled: false },
    { source: AgentSessionSource.Workflow, expectedDisabled: false },
    { source: null, expectedDisabled: false },
    { source: AgentSessionSource.ChatChannel, expectedDisabled: true },
  ])(
    'sets input disabled state for source $source',
    ({ source, expectedDisabled }) => {
      const inputProps = renderSessionChat({
        session: { id: 'session-1', source, message: [] },
      });

      expect(inputProps).toMatchObject({
        disabled: expectedDisabled,
        sendDisabled: expectedDisabled,
      });
    },
  );

  it('disables input while the selected session detail is pending', () => {
    const inputProps = renderSessionChat({
      sessionId: 'session-2',
      session: {
        id: 'session-1',
        source: AgentSessionSource.Agent,
        message: [],
      },
    });

    expect(inputProps).toMatchObject({ disabled: true, sendDisabled: true });
  });

  it('does not carry a previous channel source into a new session', () => {
    const inputProps = renderSessionChat({
      sessionId: undefined,
      isNew: true,
      session: {
        id: 'session-channel',
        source: AgentSessionSource.ChatChannel,
        message: [],
      },
    });

    expect(inputProps).toMatchObject({ disabled: false, sendDisabled: false });
  });
});
