import { render, screen } from '@testing-library/react';

import FloatingChatWidgetMarkdown from '../floating-chat-widget-markdown';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useFetchDocumentThumbnailsByIds: () => ({
    setDocumentIds: jest.fn(),
    data: {},
  }),
  useGetDocumentUrl: () => () => undefined,
}));

jest.mock('@/components/theme-provider', () => ({
  useIsDarkTheme: () => false,
}));

// Render the hover card eagerly so the resolved chunk content is assertable.
jest.mock('@/components/ui/hover-card', () => ({
  HoverCard: ({ children }: any) => (
    <div data-testid="citation">{children}</div>
  ),
  HoverCardTrigger: ({ children }: any) => (
    <span data-testid="chip">{children}</span>
  ),
  HoverCardContent: ({ children }: any) => (
    <div data-testid="popover">{children}</div>
  ),
}));

jest.mock('@radix-ui/react-hover-card', () => ({
  Portal: ({ children }: any) => <>{children}</>,
}));

jest.mock('@/components/ui/tooltip', () => ({
  Tooltip: ({ children }: any) => <>{children}</>,
  TooltipTrigger: ({ children }: any) => <>{children}</>,
  TooltipContent: ({ children }: any) => <>{children}</>,
}));

jest.mock('@/components/svg-icon', () => ({
  __esModule: true,
  default: () => <span />,
}));

jest.mock('@/components/image', () => ({
  __esModule: true,
  default: () => null,
  AuthenticatedImg: () => null,
}));

jest.mock('react-syntax-highlighter', () => ({
  __esModule: true,
  default: ({ children }: any) => <pre>{children}</pre>,
}));

jest.mock('react-syntax-highlighter/dist/esm/styles/prism', () => ({
  oneDark: {},
  oneLight: {},
}));

// react-markdown is ESM-only; feed the whole content through the same
// custom-typography component that replaces citation markers in production.
jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children, components }: any) => {
    const ReactLib = jest.requireActual('react');
    const CustomTypography = components['custom-typography'];
    return ReactLib.createElement(CustomTypography, null, String(children));
  },
}));

jest.mock('rehype-katex', () => jest.fn());
jest.mock('rehype-raw', () => jest.fn());
jest.mock('unist-util-visit-parents', () => ({ visitParents: jest.fn() }));
jest.mock('@/constants/markdown-remark-plugins', () => ({
  MarkdownRemarkPlugins: [],
}));
jest.mock('@/constants/markdown-rehype-plugins', () => ({
  RehypeSanitizeAssistantMarkdown: jest.fn(),
}));

const buildChunk = (id: string, content: string, documentId: string) => ({
  id,
  chunk_id: id,
  content,
  content_with_weight: content,
  document_id: documentId,
  doc_id: documentId,
  image_id: '',
  img_id: '',
});

const Opaque = buildChunk(
  '0000123456789012',
  'OPAQUE_CHUNK_CONTENT',
  'doc-opaque',
);
const Hex = buildChunk('a2d067519c9e0db1', 'HEX_CHUNK_CONTENT', 'doc-hex');

const buildReference = () =>
  ({
    chunks: [Opaque, Hex],
    doc_aggs: {
      'doc-opaque': {
        doc_id: 'doc-opaque',
        doc_name: 'Opaque source.txt',
        count: 1,
      },
      'doc-hex': {
        doc_id: 'doc-hex',
        doc_name: 'Hex source.txt',
        count: 1,
      },
    },
    total: 2,
  }) as any;

const popoverTexts = () =>
  screen.getAllByTestId('popover').map((popover) => popover.textContent);

describe('FloatingChatWidgetMarkdown citations', () => {
  it('keeps an all-digit opaque id as an id, not a positional index', () => {
    render(
      <FloatingChatWidgetMarkdown
        content="see [ID:0000123456789012]"
        loading={false}
        reference={buildReference()}
      />,
    );

    expect(screen.queryByText('Reference unavailable')).not.toBeInTheDocument();
    const popovers = popoverTexts();
    expect(popovers).toHaveLength(1);
    expect(popovers[0]).toContain('OPAQUE_CHUNK_CONTENT');
    expect(popovers[0]).toContain('Opaque source.txt');
  });

  it('resolves a normal hex id by id', () => {
    render(
      <FloatingChatWidgetMarkdown
        content="see [ID:a2d067519c9e0db1]"
        loading={false}
        reference={buildReference()}
      />,
    );

    const popovers = popoverTexts();
    expect(popovers).toHaveLength(1);
    expect(popovers[0]).toContain('HEX_CHUNK_CONTENT');
    expect(popovers[0]).toContain('Hex source.txt');
  });

  it('keeps short numeric markers positional', () => {
    render(
      <FloatingChatWidgetMarkdown
        content="see [ID:0]"
        loading={false}
        reference={buildReference()}
      />,
    );

    const popovers = popoverTexts();
    expect(popovers).toHaveLength(1);
    expect(popovers[0]).toContain('OPAQUE_CHUNK_CONTENT');
  });
});
