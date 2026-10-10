import { render } from '@testing-library/react';
import { sanitize } from 'hast-util-sanitize';
import React from 'react';

import { MarkdownSanitizeSchema } from '@/constants/markdown-rehype-plugins';
import { preprocessLaTeX } from '@/utils/chat';
import HighLightMarkdown from '..';

jest.mock('@/constants/markdown-remark-plugins', () => ({
  MarkdownRemarkPlugins: [],
}));

// `rehype-raw` pulls parse5's ESM graph, which jest cannot load. The
// component's rehype pipeline therefore cannot run end-to-end here. Instead we
// assert the two pieces that actually carry the security property:
//   1. the component hands react-markdown the *post*-preprocessLaTeX string,
//      so entity-encoded payloads are decoded before the tree is sanitized;
//   2. the sanitize schema used by that pipeline strips <script> and on*
//      handlers from a tree that already contains the decoded payload.
jest.mock('rehype-raw', () => jest.fn());

let markdownChildren: unknown;
jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children }: any) => {
    markdownChildren = children;
    return null;
  },
}));

jest.mock('react-syntax-highlighter', () => ({
  Prism: () => null,
}));

jest.mock('react-syntax-highlighter/dist/esm/styles/prism', () => ({
  oneDark: {},
  oneLight: {},
}));

jest.mock('rehype-katex', () => jest.fn());

jest.mock('../../theme-provider', () => ({
  useIsDarkTheme: () => false,
}));

const collect = (node: any, acc: any[] = []): any[] => {
  acc.push(node);
  (node.children ?? []).forEach((child: any) => collect(child, acc));
  return acc;
};

describe('HighLightMarkdown', () => {
  it('sanitizes the post-preprocessLaTeX payload before rendering', () => {
    const raw =
      'hello <img src=x onerror="alert(1)" /><script>alert(1)</script><b>safe</b>';

    render(React.createElement(HighLightMarkdown, null, raw));

    // preprocessLaTeX runs first; the sanitizer later sees decoded markup.
    expect(markdownChildren).toBe(preprocessLaTeX(raw));

    const tree: any = sanitize(
      {
        type: 'root',
        children: [
          {
            type: 'element',
            tagName: 'script',
            properties: {},
            children: [{ type: 'text', value: 'alert(1)' }],
          },
          {
            type: 'element',
            tagName: 'img',
            properties: { src: 'x', onError: 'alert(1)' },
            children: [],
          },
          {
            type: 'element',
            tagName: 'b',
            properties: {},
            children: [{ type: 'text', value: 'safe' }],
          },
        ],
      },
      MarkdownSanitizeSchema,
    );

    const nodes = collect(tree);

    // <script> is not in the allow-list and is dropped entirely.
    expect(nodes.some((n) => n.tagName === 'script')).toBe(false);

    // <img> is allowed, but the on* handler must be stripped.
    const img = nodes.find((n) => n.tagName === 'img');
    expect(img).toBeDefined();
    expect(img.properties.onError).toBeUndefined();

    // <b>safe</b> survives with its text.
    const b = nodes.find((n) => n.tagName === 'b');
    expect(b).toBeDefined();
    expect(b.children[0].value).toBe('safe');
  });

  it('decodes entity-encoded payloads before handing them to the renderer', () => {
    const raw =
      '&lt;img src=x onerror="alert(1)" /&gt;&lt;script&gt;alert(1)&lt;/script&gt;safe';

    render(React.createElement(HighLightMarkdown, null, raw));

    // The entity-encoded markup is decoded here, so the sanitizer downstream
    // cannot be bypassed by encoding the payload.
    const decoded = preprocessLaTeX(raw);
    expect(decoded).toContain('<img src=x onerror="alert(1)" />');
    expect(decoded).toContain('<script>alert(1)</script>');
    expect(decoded).toContain('safe');
    expect(markdownChildren).toBe(decoded);
  });
});
