import { TooltipProvider } from '@/components/ui/tooltip';
import { Operator } from '@/constants/agent';
import {
  AgentInstanceContext,
  HandleContext,
} from '@/pages/agent/context';
import { useIsPipeline } from '@/pages/agent/hooks/use-is-pipeline';
import useGraphStore from '@/pages/agent/store';
import { Position } from '@xyflow/react';
import { fireEvent, render } from '@testing-library/react';
import { HideModalContext, OperatorItemList } from './operator-item-list';

jest.mock('@/pages/agent/hooks/use-is-pipeline', () => ({
  useIsPipeline: jest.fn(),
}));

jest.mock('@/components/operator-icon', () => ({
  __esModule: true,
  default: () => null,
}));

const mockedUseIsPipeline = jest.mocked(useIsPipeline);

const createNode = (id: string, label: string) => ({
  id,
  type: 'ragNode',
  position: { x: 0, y: 0 },
  data: { label, name: id, form: {} },
});

const createEdge = (id: string, source: string, target: string) => ({
  id,
  source,
  target,
});

const handleContext = {
  nodeId: 'compiler:0',
  id: 'start',
  type: 'source' as const,
  position: Position.Right,
  isFromConnectionDrag: true,
};

function renderItemList(addCanvasNode: jest.Mock, hideModal: jest.Mock) {
  return render(
    <TooltipProvider>
      <AgentInstanceContext.Provider
        value={{ addCanvasNode } as any}
      >
        <HandleContext.Provider value={handleContext}>
          <HideModalContext.Provider value={hideModal}>
            <OperatorItemList
              operators={[Operator.TitleChunker]}
              isCustomDropdown
              mousePosition={{ x: 10, y: 10 }}
            />
          </HideModalContext.Provider>
        </HandleContext.Provider>
      </AgentInstanceContext.Provider>
    </TooltipProvider>,
  );
}

const clickFirstItem = (container: HTMLElement) =>
  fireEvent.click(container.querySelector('li')!);

describe('OperatorItemList (pipeline next-step selection)', () => {
  let addCanvasNode: jest.Mock;
  let hideModal: jest.Mock;

  beforeEach(() => {
    addCanvasNode = jest.fn(() => () => 'titleChunker:0');
    hideModal = jest.fn();
    useGraphStore.setState({ nodes: [], edges: [] });
  });

  it('does not create a node when the origin already has a downstream', () => {
    mockedUseIsPipeline.mockReturnValue(true);
    useGraphStore.setState({
      nodes: [
        createNode('compiler:0', Operator.Compiler),
        createNode('tokenizer:0', Operator.Tokenizer),
      ],
      edges: [createEdge('e1', 'compiler:0', 'tokenizer:0')],
    });

    const { container } = renderItemList(addCanvasNode, hideModal);
    clickFirstItem(container);

    expect(addCanvasNode).not.toHaveBeenCalled();
    expect(hideModal).toHaveBeenCalled();
  });

  it('creates the node while the only downstream is a pending placeholder', () => {
    mockedUseIsPipeline.mockReturnValue(true);
    useGraphStore.setState({
      nodes: [
        createNode('compiler:0', Operator.Compiler),
        createNode('placeholder:0', Operator.Placeholder),
      ],
      edges: [createEdge('e1', 'compiler:0', 'placeholder:0')],
    });

    const { container } = renderItemList(addCanvasNode, hideModal);
    clickFirstItem(container);

    expect(addCanvasNode).toHaveBeenCalledWith(
      Operator.TitleChunker,
      handleContext,
    );
    expect(hideModal).toHaveBeenCalled();
  });

  it('still creates the node on the workflow canvas where branching is allowed', () => {
    mockedUseIsPipeline.mockReturnValue(false);
    useGraphStore.setState({
      nodes: [
        createNode('compiler:0', Operator.Compiler),
        createNode('tokenizer:0', Operator.Tokenizer),
      ],
      edges: [createEdge('e1', 'compiler:0', 'tokenizer:0')],
    });

    const { container } = renderItemList(addCanvasNode, hideModal);
    clickFirstItem(container);

    expect(addCanvasNode).toHaveBeenCalled();
  });
});
