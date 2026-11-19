import { render } from '@testing-library/react';
import React from 'react';

import HighLightMarkdown from '..';

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

jest.mock('../../theme-provider', () => ({
  useIsDarkTheme: () => false,
}));

describe('HighLightMarkdown', () => {
  it('renders the actual LaTeX pipeline for a short inline formula and an inequality', () => {
    const { container } = render(
      <HighLightMarkdown>
        {String.raw`Student \(t\) and the case \(y&lt;z\).`}
      </HighLightMarkdown>,
    );

    expect(
      Array.from(
        container.querySelectorAll('.katex annotation'),
        (node) => node.textContent,
      ),
    ).toEqual(['t', 'y<z']);
  });

  it('sanitizes unsafe html before rendering', () => {
    const { container } = render(
      React.createElement(
        HighLightMarkdown,
        null,
        'hello <img src=/sample.png onerror="alert(1)" /><script>alert(1)</script><b>safe</b>',
      ),
    );

    expect(container.querySelector('b')?.textContent).toBe('safe');
    expect(container.querySelector('script')).toBeNull();
    expect(container.querySelector('img')).not.toBeNull();
    expect(container.querySelector('[onerror]')).toBeNull();
  });

  it('strips html encoded as entities (preprocessLaTeX bypass)', () => {
    // preprocessLaTeX() decodes &lt;/&gt;/&amp; back to raw HTML before
    // rehypeRaw runs. Sanitization must occur AFTER preprocessLaTeX, so
    // a payload delivered as &lt;img onerror=...&gt; cannot survive.
    const { container } = render(
      React.createElement(
        HighLightMarkdown,
        null,
        '&lt;img src=/sample.png onerror="alert(1)" /&gt;&lt;script&gt;alert(1)&lt;/script&gt;safe',
      ),
    );

    expect(container.querySelector('script')).toBeNull();
    expect(container.querySelector('img')).not.toBeNull();
    expect(container.querySelector('[onerror]')).toBeNull();
    expect(container.textContent).toContain('safe');
  });
});
