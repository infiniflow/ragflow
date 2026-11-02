import { render, screen } from '@testing-library/react';

import MarkdownContent from '..';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useFetchDocumentThumbnailsByIds: () => ({
    setDocumentIds: jest.fn(),
    data: {},
  }),
}));

jest.mock('@/hooks/use-loading-pause', () => ({
  useLoadingPause: () => false,
}));

// Only the artifact download handlers use these; mocking them keeps the
// file-util → service → i18n-config import chain out of the suite.
jest.mock('@/utils/file-util', () => ({
  downloadFileFromBlob: jest.fn(),
}));

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

// Render the highlighter as a plain <pre>: the assertions target citation
// resolution, not code highlighting.
jest.mock('react-syntax-highlighter', () => ({
  __esModule: true,
  default: ({ children }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement('pre', null, children);
  },
}));

// react-markdown and the remark/rehype packages are ESM-only and jest does
// not transform node_modules, so the real parser cannot run here. The
// production pipeline wraps every text node in <custom-typography>; this
// mock feeds the full content string through that same component, which is
// where citation markers are replaced.
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
  MarkdownRemarkPluginsLite: [],
}));
jest.mock('@/constants/markdown-rehype-plugins', () => ({
  RehypeSanitizeAssistantMarkdown: jest.fn(),
}));

// Radix HoverCard only mounts its content on hover; render eagerly so the
// resolved chunk content is assertable in the DOM.
jest.mock('@/components/ui/hover-card', () => ({
  HoverCard: ({ children }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement(
      'div',
      { 'data-testid': 'citation' },
      children,
    );
  },
  HoverCardTrigger: ({ children }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement('span', { 'data-testid': 'chip' }, children);
  },
  HoverCardContent: ({ children }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement(
      'div',
      { 'data-testid': 'popover' },
      children,
    );
  },
}));

jest.mock('@/components/svg-icon', () => ({
  __esModule: true,
  default: () => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement('span');
  },
}));

jest.mock('@/components/image', () => ({
  __esModule: true,
  default: () => null,
  AuthenticatedImg: () => null,
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

const HappyPrince = buildChunk(
  'a2d067519c9e0db1',
  'HAPPY_PRINCE_CHUNK_CONTENT',
  'doc-happy-prince',
);
const Nightingale = buildChunk(
  'b3e178620da1fc42',
  'NIGHTINGALE_CHUNK_CONTENT',
  'doc-nightingale',
);

// reference.chunks ships in retrieval order — Happy Prince ranked first —
// while the answer cites Nightingale first. The array shape is what the Go
// agent run actually sends on the wire.
const buildReference = () =>
  ({
    chunks: [HappyPrince, Nightingale],
    doc_aggs: {
      'doc-happy-prince': {
        doc_id: 'doc-happy-prince',
        doc_name: 'The Happy Prince.txt',
        count: 1,
      },
      'doc-nightingale': {
        doc_id: 'doc-nightingale',
        doc_name: 'The Nightingale and the Rose.txt',
        count: 1,
      },
    },
    total: 2,
  }) as any;

const chipTexts = () =>
  screen.getAllByTestId('chip').map((chip) => chip.textContent);
const popoverTexts = () =>
  screen.getAllByTestId('popover').map((popover) => popover.textContent);

describe('MarkdownContent citations', () => {
  it('resolves agent [ID:<chunkId>] markers by chunk id, not citation order', () => {
    render(
      <MarkdownContent
        content={
          'The same volume includes The Nightingale and the Rose [ID:b3e178620da1fc42] and the tears of the prince [ID:a2d067519c9e0db1].'
        }
        loading={false}
        reference={buildReference()}
      />,
    );

    expect(chipTexts()).toEqual(['[1]', '[2]']);
    const popovers = popoverTexts();
    expect(popovers[0]).toContain('NIGHTINGALE_CHUNK_CONTENT');
    expect(popovers[0]).toContain('The Nightingale and the Rose.txt');
    expect(popovers[1]).toContain('HAPPY_PRINCE_CHUNK_CONTENT');
    expect(popovers[1]).toContain('The Happy Prince.txt');
  });

  it('resolves numeric markers by the index they carry, not citation order', () => {
    render(
      <MarkdownContent
        content={'Cites the second chunk first [ID:1] then the first [ID:0].'}
        loading={false}
        reference={buildReference()}
      />,
    );

    expect(chipTexts()).toEqual(['[1]', '[2]']);
    const popovers = popoverTexts();
    expect(popovers[0]).toContain('NIGHTINGALE_CHUNK_CONTENT');
    expect(popovers[1]).toContain('HAPPY_PRINCE_CHUNK_CONTENT');
  });

  it('keeps an unresolvable marker as literal text once loading finished', () => {
    const { container } = render(
      <MarkdownContent
        content={'A hallucinated source [ID:ffffffffffffffff] stays text.'}
        loading={false}
        reference={buildReference()}
      />,
    );

    expect(screen.queryByTestId('chip')).toBeNull();
    expect(container.textContent).toContain('[ID:ffffffffffffffff]');
  });

  it('renders the chip without popover while the reference is still streaming', () => {
    render(
      <MarkdownContent
        content={'Sources arriving [ID:b3e178620da1fc42] soon.'}
        loading={true}
        reference={undefined}
      />,
    );

    expect(chipTexts()).toEqual(['[1]']);
    expect(screen.queryByTestId('popover')).toBeNull();
  });
});
