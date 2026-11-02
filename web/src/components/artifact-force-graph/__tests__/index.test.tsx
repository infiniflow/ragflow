import { act, render, screen } from '@testing-library/react';
import { createElement } from 'react';
import { getKnowledgeGraphNodeLabel } from '@/components/structure-graph/use-knowledge-graph-node-style';

import ArtifactForceGraph from '..';

const mockGraphRender = jest.fn();
const mockZoomToFit = jest.fn();

jest.mock('react-force-graph-2d', () => {
  const ReactLib = jest.requireActual<typeof import('react')>('react');
  const MockForceGraph = ReactLib.forwardRef<
    { zoomToFit: typeof mockZoomToFit },
    {
      graphData: { nodes: Array<{ description?: string }> };
      nodeLabel: (node: { description?: string }) => string;
    }
  >((props, ref) => {
    const { graphData, nodeLabel } = props;
    mockGraphRender(props);
    ReactLib.useImperativeHandle(ref, () => ({ zoomToFit: mockZoomToFit }));
    const label = nodeLabel(graphData.nodes[0]);
    if (!label) return null;

    // Match the graph library's HTML tooltip sink instead of rendering text.
    return ReactLib.createElement('div', {
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

function renderGraph(
  description: string,
  props: Pick<import('../types').ArtifactForceGraphProps, 'getNodeLabel'> = {},
) {
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
      ...props,
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

  it('escapes the type and description from a custom plain-text label once', () => {
    const type = '<img src=x onerror=alert(1)> &lt;type&gt;';
    const description = '<script>bad()</script> &amp; 齿轮';
    renderGraph(description, {
      getNodeLabel: (node) => getKnowledgeGraphNodeLabel({ ...node, type }),
    });

    const tooltip = screen.getByTestId('node-tooltip');
    expect(tooltip.textContent).toBe(`${type} · ${description}`);
    expect(tooltip.querySelector('img, script')).toBeNull();
  });

  it('shows the knowledge graph type even without a description', () => {
    renderGraph('', { getNodeLabel: getKnowledgeGraphNodeLabel });
    expect(screen.getByTestId('node-tooltip')).toHaveTextContent('entity');
  });

  it('keeps graph objects and fitting stable across label, pinned and hover changes', () => {
    mockZoomToFit.mockClear();
    const data: import('@/interfaces/database/dataset').IArtifactGraph = {
      entities: ['a', 'b', 'c'].map((slug) => ({
        slug,
        name: slug,
        aliases: [],
        description: slug,
        type: 'entity',
        weight: 1,
      })),
      relations: [{ from: 'a', to: 'b' }],
    };
    const getNodeColor = () => '#4e79a7';
    const { rerender } = render(
      <ArtifactForceGraph data={data} getNodeColor={getNodeColor} />,
    );
    const initial = mockGraphRender.mock.calls.at(-1)![0];
    const [a, b, c] = initial.graphData.nodes;
    act(() => initial.onEngineStop());
    expect(mockZoomToFit).toHaveBeenCalledTimes(1);

    rerender(
      <ArtifactForceGraph
        data={data}
        getNodeColor={getNodeColor}
        getNodeLabel={getKnowledgeGraphNodeLabel}
        highlightNodeId="a"
      />,
    );
    const pinned = mockGraphRender.mock.calls.at(-1)![0];
    expect(pinned.graphData).toBe(initial.graphData);
    expect(pinned.graphData.nodes[0]).toBe(a);
    expect(pinned.nodeColor(a)).toBe('#4e79a7');
    expect(pinned.nodeColor(b)).toBe('#4e79a7');
    expect(pinned.nodeColor(c)).toBe('#4e79a733');
    act(() => pinned.onEngineStop());
    expect(mockZoomToFit).toHaveBeenCalledTimes(1);

    act(() => pinned.onNodeHover(c));
    const hovered = mockGraphRender.mock.calls.at(-1)![0];
    expect(hovered.graphData).toBe(initial.graphData);
    expect(hovered.nodeColor(c)).toBe('#4e79a7');
    expect(hovered.nodeColor(a)).toBe('#4e79a733');
    act(() => hovered.onNodeHover(null));
    const restored = mockGraphRender.mock.calls.at(-1)![0];
    expect(restored.graphData).toBe(initial.graphData);
    expect(restored.nodeColor(a)).toBe('#4e79a7');
    expect(restored.nodeColor(c)).toBe('#4e79a733');
  });
});
