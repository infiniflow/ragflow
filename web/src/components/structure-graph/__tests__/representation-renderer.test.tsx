import { render } from '@testing-library/react';
import { CompilationTemplateKind } from '@/constants/compilation';
import { RepresentationRenderer } from '../representation-renderer';

const mockGraphRender = jest.fn();

jest.mock('@/components/artifact-force-graph', () => ({
  __esModule: true,
  default: (props: unknown) => {
    mockGraphRender(props);
    return null;
  },
}));
jest.mock('@/components/ui/tree-view', () => ({ TreeView: () => null }));
jest.mock('../mindmap-g6-graph', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('../timeline-x6-graph', () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const Template = {
  kind: CompilationTemplateKind.KnowledgeGraph,
  template_id: 'test',
  template_name: 'test',
  entities: [
    { id: 'Ada', name: 'Ada', type: 'Person', description: 'A mathematician' },
  ],
  relations: [],
};

describe('KnowledgeGraph styling scope', () => {
  it('keeps graph data and style callbacks stable when the highlighted node changes', () => {
    const { rerender } = render(<RepresentationRenderer template={Template} />);
    const initial = mockGraphRender.mock.calls.at(-1)![0];
    expect(initial.getNodeColor).toEqual(expect.any(Function));
    expect(initial.getNodeLabel(initial.data.entities[0])).toBe(
      'Person · A mathematician',
    );
    expect(initial.data.entities[0].description).toBe('A mathematician');

    rerender(
      <RepresentationRenderer template={Template} highlightNodeId="Ada" />,
    );
    const highlighted = mockGraphRender.mock.calls.at(-1)![0];
    expect(highlighted.data).toBe(initial.data);
    expect(highlighted.getNodeColor).toBe(initial.getNodeColor);
    expect(highlighted.getNodeLabel).toBe(initial.getNodeLabel);
    expect(highlighted.highlightNodeId).toBe('Ada');
  });

  it.each([
    CompilationTemplateKind.SessionGraph,
    CompilationTemplateKind.Empty,
  ])('leaves %s on the default graph colors and tooltip', (kind) => {
    render(<RepresentationRenderer template={{ ...Template, kind }} />);
    const props = mockGraphRender.mock.calls.at(-1)![0];
    expect(props.getNodeColor).toBeUndefined();
    expect(props.getNodeLabel).toBeUndefined();
  });
});
