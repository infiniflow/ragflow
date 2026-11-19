import { render, screen } from '@testing-library/react';
import { Textarea } from '../textarea';

// adjustHeight collapses the textarea to `auto` synchronously and sets the
// measured height in the next frame, so `auto` marks a re-measure.
const renderTextarea = (value: string) => (
  <Textarea
    data-testid="box"
    value={value}
    onChange={jest.fn()}
    // As in the chat input; jsdom has no ResizeObserver for the manual resize.
    resize="none"
    autoSize={{ minRows: 1, maxRows: 8 }}
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
});
