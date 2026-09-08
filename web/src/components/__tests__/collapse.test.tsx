import { fireEvent, render, screen } from '@testing-library/react';
import { Collapse } from '../collapse';

describe('Collapse', () => {
  it('mounts and unmounts content at render time when open is controlled', () => {
    const { rerender } = render(
      <Collapse title="Section" open={false}>
        <div>inner content</div>
      </Collapse>,
    );
    expect(screen.queryByText('inner content')).not.toBeInTheDocument();

    rerender(
      <Collapse title="Section" open={true}>
        <div>inner content</div>
      </Collapse>,
    );
    expect(screen.getByText('inner content')).toBeInTheDocument();

    rerender(
      <Collapse title="Section" open={false}>
        <div>inner content</div>
      </Collapse>,
    );
    expect(screen.queryByText('inner content')).not.toBeInTheDocument();
  });

  it('notifies onOpenChange when the trigger is clicked in controlled mode', () => {
    const handleOpenChange = jest.fn();
    render(
      <Collapse title="Section" open={false} onOpenChange={handleOpenChange}>
        <div>inner content</div>
      </Collapse>,
    );

    fireEvent.click(screen.getByText('Section'));

    expect(handleOpenChange).toHaveBeenCalledWith(true);
  });

  it('toggles by itself when open is not provided', () => {
    render(
      <Collapse title="Section" defaultOpen>
        <div>inner content</div>
      </Collapse>,
    );
    expect(screen.getByText('inner content')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Section'));

    expect(screen.queryByText('inner content')).not.toBeInTheDocument();
  });

  it('follows a changing defaultOpen when uncontrolled', () => {
    const { rerender } = render(
      <Collapse title="Section" defaultOpen={false}>
        <div>inner content</div>
      </Collapse>,
    );
    expect(screen.queryByText('inner content')).not.toBeInTheDocument();

    rerender(
      <Collapse title="Section" defaultOpen={true}>
        <div>inner content</div>
      </Collapse>,
    );

    expect(screen.getByText('inner content')).toBeInTheDocument();
  });
});
