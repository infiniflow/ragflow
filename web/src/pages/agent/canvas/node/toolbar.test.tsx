import { render, screen } from '@testing-library/react';
import { CanvasReadonlyContext } from '../../context';
import { ToolBar } from './toolbar';

// TooltipContent relies on React Flow's NodeToolbar portal, which jsdom
// cannot satisfy; flatten the tooltip wrappers into plain divs so the
// assertions focus on whether ToolBar renders its action bar at all.
jest.mock('@/components/xyflow/tooltip-node', () => ({
  TooltipNode: ({ children }: any) => <div>{children}</div>,
  TooltipTrigger: ({ children }: any) => <div>{children}</div>,
  TooltipContent: ({ children }: any) => (
    <div data-testid="node-toolbar">{children}</div>
  ),
}));

jest.mock('../../hooks', () => ({
  useDuplicateNode: () => jest.fn(),
}));

function renderToolBar(readOnly: boolean) {
  return render(
    <CanvasReadonlyContext.Provider value={readOnly}>
      <ToolBar selected={false} id="node-1" label="Parser">
        <div>node body</div>
      </ToolBar>
    </CanvasReadonlyContext.Provider>,
  );
}

describe('ToolBar', () => {
  it('shows the run/copy/delete action bar on an editable canvas', () => {
    renderToolBar(false);

    expect(screen.getByTestId('node-toolbar')).toBeInTheDocument();
    expect(
      screen.getByTestId('node-toolbar').querySelector('[data-play]'),
    ).not.toBeNull();
  });

  it('hides the whole action bar on a read-only canvas', () => {
    renderToolBar(true);

    expect(screen.queryByTestId('node-toolbar')).not.toBeInTheDocument();
    // The node body itself must still render so the canvas stays viewable.
    expect(screen.getByText('node body')).toBeInTheDocument();
  });
});
