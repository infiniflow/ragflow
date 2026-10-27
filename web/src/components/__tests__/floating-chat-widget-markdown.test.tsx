import { fireEvent, render, screen } from '@testing-library/react';
import FloatingChatWidgetMarkdown from '../floating-chat-widget-markdown';

type IReference = import('@/interfaces/database/chat').IReference;
type IReferenceChunk = import('@/interfaces/database/chat').IReferenceChunk;

jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children, components }: any) => {
    const Typography = components['custom-typography'];
    return <Typography>{children}</Typography>;
  },
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-document-request', () => ({
  useFetchDocumentThumbnailsByIds: () => ({
    setDocumentIds: jest.fn(),
    data: {},
  }),
  useGetDocumentUrl: () => (id: string) => `/document/${id}`,
}));
jest.mock('../theme-provider', () => ({ useIsDarkTheme: () => false }));
jest.mock('../image', () => ({
  __esModule: true,
  default: () => null,
  AuthenticatedImg: () => null,
}));
jest.mock('../svg-icon', () => ({ __esModule: true, default: () => null }));
jest.mock('../ui/hover-card', () => {
  const Wrapper = ({ children }: any) => <div>{children}</div>;
  return {
    HoverCard: Wrapper,
    HoverCardContent: ({ children }: any) => (
      <div data-testid="popover">{children}</div>
    ),
    HoverCardTrigger: Wrapper,
  };
});
jest.mock('@radix-ui/react-hover-card', () => ({
  Portal: ({ children }: any) => <>{children}</>,
}));
jest.mock('../ui/tooltip', () => {
  const Wrapper = ({ children }: any) => <div>{children}</div>;
  return { Tooltip: Wrapper, TooltipContent: Wrapper, TooltipTrigger: Wrapper };
});
jest.mock('@/utils/chat', () => ({
  ...jest.requireActual('@/utils/citation-utils'),
  currentReg: jest.requireActual('@/utils/citation-utils').citationMarkerReg,
  preprocessLaTeX: (text: string) => text,
  replaceRetrievingToSection: (text: string) => text,
  replaceTextByOldReg: (text: string) => text,
  replaceThinkToSection: (text: string) => text,
  showImage: () => false,
}));
jest.mock('@/constants/markdown-remark-plugins', () => ({
  MarkdownRemarkPlugins: [],
}));
jest.mock('@/constants/markdown-rehype-plugins', () => ({
  RehypeSanitizeAssistantMarkdown: () => undefined,
}));
jest.mock('rehype-katex', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('rehype-raw', () => ({ __esModule: true, default: jest.fn() }));
jest.mock('unist-util-visit-parents', () => ({ visitParents: jest.fn() }));
jest.mock('react-syntax-highlighter', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('react-syntax-highlighter/dist/esm/styles/prism', () => ({
  oneDark: {},
  oneLight: {},
}));

// Synthetic references deliberately use different IDs and documents.
const FirstChunk: IReferenceChunk = {
  id: 'a2d067519c9e0db1',
  document_id: 'first-document',
  document_name: 'first-source.pdf',
  content: null,
  dataset_id: 'dataset',
  image_id: '',
  similarity: 1,
  vector_similarity: 1,
  term_similarity: 1,
  positions: [],
};
const SecondChunk: IReferenceChunk = {
  ...FirstChunk,
  id: 'b3e178620da1fc42',
  document_id: 'second-document',
  document_name: 'second-source.pdf',
};

function renderCitation(
  marker: string,
  chunks: IReferenceChunk[] = [FirstChunk, SecondChunk],
  loading = false,
) {
  const clickDocumentButton = jest.fn();
  const reference: IReference = {
    chunks,
    doc_aggs: chunks.map((chunk) => ({
      count: 1,
      doc_id: chunk.document_id,
      doc_name: chunk.document_name,
    })),
    total: chunks.length,
  };
  const result = render(
    <FloatingChatWidgetMarkdown
      content={`Answer ${marker}`}
      reference={reference}
      loading={loading}
      clickDocumentButton={clickDocumentButton}
    />,
  );
  return { ...result, clickDocumentButton };
}

describe('floating widget citation resolution', () => {
  it.each(['[ID:0]', '[0]', '[ID:٠]', '[ID:۰]'])(
    'resolves the positional marker %s',
    (marker) => {
      renderCitation(marker);
      expect(
        screen.getByRole('button', { name: FirstChunk.document_name }),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole('button', { name: SecondChunk.document_name }),
      ).not.toBeInTheDocument();
    },
  );

  it('resolves an agent chunk ID independently of retrieval order', () => {
    const { clickDocumentButton } = renderCitation(`[ID:${SecondChunk.id}]`);
    const sourceButton = screen.getByRole('button', {
      name: SecondChunk.document_name,
    });
    expect(sourceButton).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: FirstChunk.document_name }),
    ).not.toBeInTheDocument();
    fireEvent.click(sourceButton);
    expect(clickDocumentButton).toHaveBeenCalledWith(
      SecondChunk.document_id,
      SecondChunk,
    );
  });

  it('resolves a legacy chunk_id field', () => {
    renderCitation(`[ID:${SecondChunk.id}]`, [
      {
        ...SecondChunk,
        id: undefined,
        chunk_id: SecondChunk.id,
      } as unknown as IReferenceChunk,
    ]);
    expect(
      screen.getByRole('button', { name: SecondChunk.document_name }),
    ).toBeInTheDocument();
  });

  it.each(['[ID:3]', '[ID:ffffffffffffffff]'])(
    'keeps the unavailable-source fallback for %s',
    (marker) => {
      renderCitation(marker);
      expect(screen.getByText('Reference unavailable')).toBeInTheDocument();
    },
  );

  it('keeps the loading fallback until sources arrive', () => {
    renderCitation('[ID:0]', [], true);
    expect(screen.getByText('chat.searching')).toBeInTheDocument();
  });
});

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
