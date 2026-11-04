import { fireEvent, render, screen } from '@testing-library/react';
import useGraphStore from '../store';
import { CanvasIssue, CanvasIssueType } from '../utils/canvas-checklist';
import { CanvasChecklist } from './canvas-checklist';

// Radix Popover measures via ResizeObserver, which jsdom does not provide.
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverMock;

const issue: CanvasIssue = {
  nodeId: 'Message:m1',
  nodeName: 'Message',
  operatorLabel: 'Message',
  type: CanvasIssueType.MissingRequired,
  messageKey: 'flow.messageMsg',
};

describe('CanvasChecklist', () => {
  beforeEach(() => {
    useGraphStore.setState({ nodeFocusRequest: null });
  });

  it('posts a node focus request with the form opened for variable issues', () => {
    render(<CanvasChecklist issues={[issue]} />);

    fireEvent.click(screen.getByTestId('canvas-checklist'));
    fireEvent.click(screen.getByTestId('canvas-checklist-issue'));

    expect(useGraphStore.getState().nodeFocusRequest).toEqual({
      nodeId: 'Message:m1',
      toolId: undefined,
      openForm: true,
      nonce: expect.any(Number),
    });
  });

  it('only highlights the node for orphan issues', () => {
    render(
      <CanvasChecklist issues={[{ ...issue, type: CanvasIssueType.Orphan }]} />,
    );

    fireEvent.click(screen.getByTestId('canvas-checklist'));
    fireEvent.click(screen.getByTestId('canvas-checklist-issue'));

    expect(useGraphStore.getState().nodeFocusRequest).toEqual({
      nodeId: 'Message:m1',
      toolId: undefined,
      openForm: false,
      nonce: expect.any(Number),
    });
  });

  it('groups issues from different tools of the same agent separately', () => {
    const toolIssue = (toolId: string, nodeName: string): CanvasIssue => ({
      nodeId: 'Tool:t1',
      toolId,
      nodeName,
      operatorLabel: 'Retrieval',
      type: CanvasIssueType.MissingRequired,
      messageKey: 'flow.retrievalDatasetMissing',
    });
    render(
      <CanvasChecklist
        issues={[
          toolIssue('tool-1', 'Agent / DocsA'),
          toolIssue('tool-2', 'Agent / DocsB'),
        ]}
      />,
    );

    fireEvent.click(screen.getByTestId('canvas-checklist'));

    expect(screen.getByText('Agent / DocsA')).toBeInTheDocument();
    expect(screen.getByText('Agent / DocsB')).toBeInTheDocument();
  });
});
