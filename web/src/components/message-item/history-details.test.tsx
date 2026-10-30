import { render, screen } from '@testing-library/react';
import { MessageType } from '@/constants/chat';
import { buildMessageListWithUuid } from '@/utils/chat';
import MessageItem from './index';

jest.mock('../theme-provider', () => ({
  useTheme: () => ({ theme: 'light' }),
}));
jest.mock('../markdown-content', () => ({
  __esModule: true,
  default: ({ content }: { content: string }) => <div>{content}</div>,
}));
jest.mock('../svg-icon', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('../ragflow-avatar', () => ({ RAGFlowAvatar: () => null }));
jest.mock('../document-download-button', () => ({
  DocumentDownloadButton: () => null,
}));
jest.mock('../next-message-item/reference-image-list', () => ({
  ReferenceImageList: () => null,
}));
jest.mock('../next-message-item/uploaded-message-files', () => ({
  UploadedMessageFiles: () => null,
}));
jest.mock('./group-button', () => ({
  AssistantGroupButton: ({ prompt }: { prompt?: string }) => (
    <div data-testid="prompt-details">{prompt}</div>
  ),
  UserGroupButton: () => null,
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values: Record<string, number | string>) =>
      key === 'tokensSpent'
        ? `${values.total} tokens (${values.prompt} input, ${values.completion} output)`
        : `${values.seconds} seconds`,
  }),
}));

describe('History answer details', () => {
  it('preserves API message details for the prompt action and usage footer', () => {
    const [message] = buildMessageListWithUuid([
      {
        id: 'turn-1',
        role: MessageType.Assistant,
        content: 'Saved answer',
        prompt: 'Saved system prompt',
        usage: {
          prompt_tokens: 896,
          completion_tokens: 71,
          total_tokens: 967,
        },
        elapsed_ms: 4168.774506,
      },
    ]);

    render(
      <MessageItem
        item={message}
        reference={{ chunks: [], doc_aggs: [], total: 0 }}
        index={2}
      />,
    );

    expect(screen.getByTestId('prompt-details')).toHaveTextContent(
      'Saved system prompt',
    );
    expect(screen.getByTestId('turn-usage')).toHaveTextContent(
      '967 tokens (896 input, 71 output) · 4.2 seconds',
    );
  });

  it('renders old answers without a usage footer', () => {
    render(
      <MessageItem
        item={{
          id: 'old-turn',
          role: MessageType.Assistant,
          content: 'Old answer',
        }}
        reference={{ chunks: [], doc_aggs: [], total: 0 }}
        index={2}
      />,
    );

    expect(screen.getByText('Old answer')).toBeInTheDocument();
    expect(screen.queryByTestId('turn-usage')).not.toBeInTheDocument();
  });
});
