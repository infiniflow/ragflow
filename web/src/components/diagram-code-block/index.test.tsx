import { render, screen, waitFor } from '@testing-library/react';
import mermaid from 'mermaid';
import { DiagramCodeBlock, getDiagramKind } from '.';

jest.mock('mermaid', () => ({
  __esModule: true,
  default: {
    initialize: jest.fn(),
    render: jest.fn(),
  },
}));

const mockedMermaidRender = mermaid.render as jest.Mock;

describe('DiagramCodeBlock', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it.each([
    ['mermaid', 'mermaid'],
    ['meraid', 'mermaid'],
    ['plantuml', 'plantuml'],
    ['puml', 'plantuml'],
    ['startuml', 'plantuml'],
  ] as const)('maps %s to %s', (language, expected) => {
    expect(getDiagramKind(language)).toBe(expected);
  });

  it('renders and sanitizes Mermaid SVG', async () => {
    mockedMermaidRender.mockResolvedValue({
      svg: '<svg><text>Flow</text><image href="https://example.com/leak.png"/><script>alert(1)</script></svg>',
      diagramType: 'flowchart-v2',
      bindFunctions: undefined,
    });

    const { container } = render(
      <DiagramCodeBlock language="mermaid" source="flowchart LR; A --> B" />,
    );

    await waitFor(() => {
      expect(screen.getByText('Flow')).toBeInTheDocument();
    });
    expect(container.querySelector('script')).not.toBeInTheDocument();
    expect(container.querySelector('image')).not.toBeInTheDocument();
    expect(mockedMermaidRender).toHaveBeenCalledWith(
      expect.stringMatching(/^ragflow-mermaid-/),
      'flowchart LR; A --> B',
    );
  });

  it('posts PlantUML source to the local renderer and sanitizes its SVG', async () => {
    const fetchMock = jest.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        '<svg><text>Sequence</text><image href="https://example.com/leak.png"/><script>bad()</script></svg>',
        {
          status: 200,
          headers: { 'Content-Type': 'image/svg+xml' },
        },
      ),
    );
    const source = '@startuml\nAlice -> Bob: hello\n@enduml';

    const { container } = render(
      <DiagramCodeBlock language="plantuml" source={source} />,
    );

    await waitFor(() => {
      expect(screen.getByText('Sequence')).toBeInTheDocument();
    });
    expect(container.querySelector('script')).not.toBeInTheDocument();
    expect(container.querySelector('image')).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      '/plantuml/svg/',
      expect.objectContaining({
        method: 'POST',
        body: source,
      }),
    );
  });

  it('shows Mermaid source when rendering fails', async () => {
    const source = 'not valid Mermaid source';
    mockedMermaidRender.mockRejectedValue(new Error('Mermaid render failed'));

    render(<DiagramCodeBlock language="mermaid" source={source} />);

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Mermaid render failed',
    );
    expect(screen.getByText(source)).toBeInTheDocument();
  });

  it('shows PlantUML source when rendering fails', async () => {
    jest
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response('render failed', { status: 400 }));
    const source = '@startuml\ninvalid source\n@enduml';

    render(<DiagramCodeBlock language="plantuml" source={source} />);

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'PlantUML renderer returned HTTP 400',
    );
    expect(screen.getByText(/invalid source/)).toBeInTheDocument();
  });

  it('does not add an empty source block when no diagram code is available', async () => {
    mockedMermaidRender.mockRejectedValue(new Error('Mermaid render failed'));

    const { container } = render(
      <DiagramCodeBlock language="mermaid" source=" " />,
    );

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Mermaid render failed',
    );
    expect(container.querySelector('pre')).not.toBeInTheDocument();
  });
});
