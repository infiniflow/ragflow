import { render, screen } from '@testing-library/react';
import { Textarea } from '../textarea';

// adjustHeight collapses the textarea to `auto` synchronously and sets the
// measured height in the next frame, so `auto` marks a re-measure.
const renderTextarea = (value: string, minRows = 1) => (
  <Textarea
    data-testid="box"
    value={value}
    onChange={jest.fn()}
    // As in the chat input; jsdom has no ResizeObserver for the manual resize.
    resize="none"
    autoSize={{ minRows, maxRows: 8 }}
  />
);

describe('Textarea autoSize', () => {
  beforeEach(() => {
    jest.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('does not re-measure when only the autoSize object is recreated', () => {
    const { rerender } = render(renderTextarea('hello'));
    const box = screen.getByTestId('box');
    box.style.height = '48px';

    rerender(renderTextarea('hello'));

    expect(box.style.height).toBe('48px');
  });

  it('re-measures when the value changes', () => {
    const { rerender } = render(renderTextarea('hello'));
    const box = screen.getByTestId('box');
    box.style.height = '48px';

    rerender(renderTextarea('hello world'));

    expect(box.style.height).toBe('auto');
  });

  it('re-measures when minRows changes', () => {
    const { rerender } = render(renderTextarea('hello', 1));
    const box = screen.getByTestId('box');
    box.style.height = '48px';

    rerender(renderTextarea('hello', 1));
    expect(box.style.height).toBe('48px');

    rerender(renderTextarea('hello', 4));
    expect(box.style.height).toBe('auto');
  });

  it('re-measures when rows changes alongside an unchanged minRows', () => {
    const renderWithRows = (rows: number) => (
      <Textarea
        data-testid="box"
        value="hello"
        onChange={jest.fn()}
        resize="none"
        rows={rows}
        autoSize={{ minRows: 1, maxRows: 8 }}
      />
    );
    const { rerender } = render(renderWithRows(2));
    const box = screen.getByTestId('box');
    expect(box.getAttribute('rows')).toBe('2');
    box.style.height = '48px';

    rerender(renderWithRows(4));

    expect(box.getAttribute('rows')).toBe('4');
    expect(box.style.height).toBe('auto');
  });
});
