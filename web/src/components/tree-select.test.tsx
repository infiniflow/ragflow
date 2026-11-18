import { fireEvent, render, screen, within } from '@testing-library/react';
import { TreeSelect } from './tree-select';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const OriginalResizeObserver = globalThis.ResizeObserver;

beforeAll(() => {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

afterAll(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
});

describe('TreeSelect', () => {
  it('renders an optional header inside the open dropdown before search', () => {
    render(
      <TreeSelect
        data={[{ id: 'model-1', title: 'Model one' }]}
        showSearch
        dropdownHeader={<div>Mode header</div>}
      />,
    );

    expect(screen.queryByText('Mode header')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button'));

    const dropdown = screen.getByRole('dialog');
    const header = within(dropdown).getByText('Mode header');
    const search = within(dropdown).getByPlaceholderText('common.search');
    expect(
      header.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('supports controlled open state and closes after leaf selection', () => {
    const onChange = jest.fn();
    const onOpenChange = jest.fn();
    const data = [{ id: 'model-1', title: 'Model one' }];
    const { rerender } = render(
      <TreeSelect
        data={data}
        open={false}
        onOpenChange={onOpenChange}
        onChange={onChange}
      />,
    );

    const trigger = screen.getByRole('button');
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(trigger);
    expect(onOpenChange).toHaveBeenLastCalledWith(true);
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    rerender(
      <TreeSelect
        data={data}
        open
        onOpenChange={onOpenChange}
        onChange={onChange}
      />,
    );
    fireEvent.click(within(screen.getByRole('dialog')).getByText('Model one'));
    expect(onChange).toHaveBeenCalledWith('model-1');
    expect(onOpenChange).toHaveBeenLastCalledWith(false);

    rerender(
      <TreeSelect
        data={data}
        open={false}
        onOpenChange={onOpenChange}
        onChange={onChange}
      />,
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
