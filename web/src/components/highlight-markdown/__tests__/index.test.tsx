import { render } from '@testing-library/react';
import React from 'react';

import HighLightMarkdown from '..';

jest.mock('@/constants/markdown-remark-plugins', () => ({
  MarkdownRemarkPlugins: [],
}));

jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children }: any) => {
    const react = jest.requireActual('react');
    return react.createElement('div', null, children);
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

    expect(container.textContent).toContain('hello');
    expect(container.textContent).toContain('<b>safe</b>');
    expect(container.querySelector('script')).toBeNull();
    expect(container.querySelector('img')).toBeNull();
  });
});
