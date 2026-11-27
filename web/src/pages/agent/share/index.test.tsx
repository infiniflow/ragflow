import { Routes as AppRoutes } from '@/routes';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import AgentShare from './index';

jest.mock('@/components/message-input/next', () => ({
  NextMessageInput: () => <textarea aria-label="Message" />,
}));
jest.mock('@/components/next-message-item', () => ({
  __esModule: true,
  default: ({ item }: { item: { content: string } }) => <p>{item.content}</p>,
}));
jest.mock('@/components/pdf-drawer', () => () => null);
jest.mock('@/components/pdf-drawer/hooks', () => ({
  useClickDrawer: () => ({ visible: false }),
}));
jest.mock('@/components/theme-provider', () => ({
  useSyncThemeFromParams: jest.fn(),
}));
jest.mock('@/hooks/logic-hooks', () => ({
  useFetchAppConf: () => ({ appName: 'RAGFlow' }),
}));
jest.mock('@/hooks/use-agent-request', () => ({
  useFetchExternalAgentInputs: () => ({ data: { title: 'Shared agent' } }),
  useUploadAgentFileWithProgress: () => ({ loading: false }),
}));
jest.mock('@/locales/config', () => ({ __esModule: true, default: {} }));
jest.mock('@/pages/agent/debug-content', () => () => null);
jest.mock('./parameter-dialog', () => ({ ParameterDialog: () => null }));
jest.mock('../hooks/use-chat-logic', () => ({
  useAwaitComponentData: () => ({ isWaiting: false }),
}));
jest.mock('../constant', () => ({
  AgentDialogueMode: { Task: 'task' },
}));

const mockMessageContainerRef = { current: null as HTMLDivElement | null };

jest.mock('@/pages/agent/chat/use-send-agent-message', () => ({
  useSendAgentMessage: () => ({
    value: '',
    derivedMessages: [{ id: 'question', role: 'user', content: 'Hello' }],
    messageContainerRef: mockMessageContainerRef,
    virtualizer: {
      getTotalSize: () => 160,
      getVirtualItems: () => [{ index: 0, key: 'question', start: 0 }],
      measureElement: jest.fn(),
    },
    findReferenceByMessageId: jest.fn(),
  }),
}));

it('gives the fullscreen agent transcript a width below the desktop breakpoint', () => {
  render(
    <MemoryRouter
      initialEntries={[
        `${AppRoutes.AgentShare}?shared_id=agent-token&from=agent`,
      ]}
    >
      <Routes>
        <Route path={AppRoutes.AgentShare} element={<AgentShare />} />
      </Routes>
    </MemoryRouter>,
  );

  const transcript = mockMessageContainerRef.current;
  expect(transcript).toContainElement(screen.getByText('Hello'));
  expect(transcript).toHaveClass('w-full', 'md:w-5/6');
});
