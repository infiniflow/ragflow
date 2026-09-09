import { render } from '@testing-library/react';
import React from 'react';

import HighLightMarkdown from '..';

jest.mock('@/constants/markdown-remark-plugins', () => ({
  MarkdownRemarkPlugins: [],
}));

// Coderabbit MAJOR #3486038797: the previous mock rendered react-markdown's
// output as a plain <div> containing `children` as text, so the spec never
// exercised rehypeRaw or the post-preprocessLaTeX sanitization path. With
// that mock, `<b>safe</b>` was just text inside a div, and an entity-encoded
// `<img onerror=...>` payload would never reach the DOM no matter what the
// component did — masking the exact bypass DOMPurify is meant to catch.
//
// We mock react-markdown to render `children` as raw HTML (via
// dangerouslySetInnerHTML). This mimics the real pipeline: if the component
// fails to sanitize (e.g. sanitizes BEFORE preprocessLaTeX), the unsafe HTML
// will reach this mock and be inserted into the DOM, failing the assertions.
jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children }: any) => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement('div', {
      dangerouslySetInnerHTML: { __html: children },
    });
  },
}));

jest.mock('react-syntax-highlighter', () => ({
  Prism: ({ children }: any) => {
    const react = jest.requireActual('react');
    return react.createElement('pre', null, children);
  },
}));

jest.mock('react-syntax-highlighter/dist/esm/styles/prism', () => ({
  oneDark: {},
  oneLight: {},
}));

jest.mock('rehype-katex', () => jest.fn());
jest.mock('rehype-raw', () => jest.fn());

jest.mock('../../theme-provider', () => ({
  useIsDarkTheme: () => false,
}));

describe('HighLightMarkdown', () => {
  it('sanitizes unsafe html before rendering', () => {
    const { container } = render(
      React.createElement(
        HighLightMarkdown,
        null,
        'hello <img src=x onerror="alert(1)" /><script>alert(1)</script><b>safe</b>',
      ),
    );

    // <b>safe</b> is allowed by DOMPurify default profile, so it should
    // render as a real <b> element with text "safe".
    expect(container.querySelector('b')?.textContent).toBe('safe');

    // <script> is removed entirely by DOMPurify.
    expect(container.querySelector('script')).toBeNull();

    // <img> is kept but its dangerous handler attribute must be stripped.
    // (DOMPurify default profile removes on* event attributes.)
    const imgs = container.querySelectorAll('img');
    imgs.forEach((img) => {
      expect(img.getAttribute('onerror')).toBeNull();
    });
  });

  it('strips html encoded as entities (preprocessLaTeX bypass)', () => {
    // preprocessLaTeX() decodes &lt;/&gt;/&amp; back to raw HTML before
    // rehypeRaw runs. Sanitization must occur AFTER preprocessLaTeX, so
    // a payload delivered as &lt;img onerror=...&gt; cannot survive.
    const { container } = render(
      React.createElement(
        HighLightMarkdown,
        null,
        '&lt;img src=x onerror="alert(1)" /&gt;&lt;script&gt;alert(1)&lt;/script&gt;safe',
      ),
    );

    // <script> entirely removed.
    expect(container.querySelector('script')).toBeNull();
    // <img> kept (allowed by default profile) but onerror must be stripped.
    container.querySelectorAll('img').forEach((img) => {
      expect(img.getAttribute('onerror')).toBeNull();
    });
    // The literal word "safe" should still be visible.
    expect(container.textContent).toContain('safe');
  });
});
