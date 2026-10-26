import { render } from '@testing-library/react';
import React from 'react';

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

// Render the highlighter as a plain <pre>: the assertion targets the code
// string the component hands down, not the highlighting itself.
jest.mock('react-syntax-highlighter', () => ({
  __esModule: true,
  default: ({ children }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement('pre', null, children);
  },
}));

// react-markdown and the remark/rehype packages are ESM-only and jest does
// not transform node_modules, so the real parser cannot run here. This mock
// reproduces the one behavior the fix depends on: for code content the
// parser passes the source string through literally, entities intact.
jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children, components }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement(
      'pre',
      null,
      ReactLib.createElement(
        components.code,
        { className: 'language-text' },
        String(children),
      ),
    );
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

describe('MarkdownContent', () => {
  it('renders > in code blocks instead of the &gt; entity', () => {
    // The message pipeline escapes standalone angle brackets and restores
    // them as entities; code content must decode them back at render time.
    const { container } = render(
      <MarkdownContent
        content={'```text\n│ 12,000 > 5,000 → 同意并向上推送\n```'}
        loading={false}
        reference={undefined as any}
      />,
    );

    expect(container.textContent).toContain('12,000 > 5,000');
    expect(container.textContent).not.toContain('&gt;');
  });
});
