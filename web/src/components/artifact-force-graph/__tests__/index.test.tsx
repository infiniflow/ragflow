import { render, screen } from '@testing-library/react';
import { createElement } from 'react';

import ArtifactForceGraph from '..';

jest.mock('react-force-graph-2d', () => {
  const ReactLib = jest.requireActual<typeof import('react')>('react');
  const MockForceGraph = ReactLib.forwardRef<
    HTMLDivElement,
    {
      graphData: { nodes: Array<{ description?: string }> };
      nodeLabel: (node: { description?: string }) => string;
    }
  >(({ graphData, nodeLabel }, ref) => {
    const label = nodeLabel(graphData.nodes[0]);
    if (!label) return null;

    // Match the graph library's HTML tooltip sink instead of rendering text.
    return ReactLib.createElement('div', {
      ref,
      'data-testid': 'node-tooltip',
      dangerouslySetInnerHTML: { __html: label },
    });
  });
  MockForceGraph.displayName = 'MockForceGraph';
  return { __esModule: true, default: MockForceGraph };
});

jest.mock('../use-container-dimensions', () => ({
  useContainerDimensions: () => ({ width: 800, height: 600 }),
}));

jest.mock('../use-center-gravity', () => ({
  useCenterGravity: jest.fn(),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function renderGraph(description: string) {
  return render(
    createElement(ArtifactForceGraph, {
      data: {
        entities: [
          {
            slug: 'entity/齿轮',
            name: '齿轮',
            aliases: [],
            description,
            type: 'entity',
            weight: 1,
          },
        ],
        relations: [],
      },
    }),
  );
}

describe('ArtifactForceGraph node tooltip', () => {
  it.each([
    ['HTML-like text', '<script>alert("bad")</script><b>齿轮</b> & text'],
    ['literal HTML entities', '&lt;b&gt;齿轮&lt;/b&gt; &amp;'],
    ['plain Chinese', '齿轮是海雾镇钟楼报时机械的核心零件。'],
  ])('shows %s as plain text', (_, description) => {
    renderGraph(description);

    const tooltip = screen.getByTestId('node-tooltip');
    expect(tooltip.textContent).toBe(description);
    expect(tooltip.querySelector('script, b')).toBeNull();
  });

  it('hides the tooltip for an empty description', () => {
    renderGraph('');

    expect(screen.queryByTestId('node-tooltip')).toBeNull();
  });
});
