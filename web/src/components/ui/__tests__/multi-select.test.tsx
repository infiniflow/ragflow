import { fireEvent, render, screen } from '@testing-library/react';
import { MultiSelect } from '../multi-select';

beforeAll(() => {
  // jsdom does not implement these browser APIs used by cmdk/radix.
  Element.prototype.scrollIntoView = jest.fn();
  (globalThis as Record<string, unknown>).ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

const options = [
  { label: 'Chat', value: 'chat' },
  { label: 'VLM', value: 'vlm' },
  { label: 'OCR', value: 'ocr' },
];

function renderOpenedMultiSelect(onValueChange = jest.fn()) {
  const utils = render(
    <MultiSelect
      options={options}
      defaultValue={['chat', 'vlm']}
      onValueChange={onValueChange}
      placeholder="Model type"
    />,
  );

  fireEvent.click(
    utils.container.querySelector('button') as HTMLButtonElement,
  );

  const root = document.querySelector('[cmdk-root]') as HTMLElement;
  expect(root).not.toBeNull();

  const input = root.querySelector('[cmdk-input]') as HTMLInputElement;
  const items = () =>
    Array.from(root.querySelectorAll<HTMLElement>('[cmdk-item]'));
  const itemByText = (text: string) => {
    const item = items().find((element) =>
      element.textContent?.includes(text),
    );
    expect(item).toBeDefined();
    return item as HTMLElement;
  };

  // (Select all), Chat, VLM, OCR, Clear, Close
  expect(items()).toHaveLength(6);

  return { onValueChange, input, items, itemByText };
}

function pressArrowDown(input: HTMLElement, times: number) {
  for (let index = 0; index < times; index++) {
    fireEvent.keyDown(input, { key: 'ArrowDown' });
  }
}

describe('MultiSelect keyboard interaction', () => {
  it('keeps an already-selected option selected and closes the dropdown on Enter', () => {
    const onValueChange = jest.fn();
    const { input, itemByText } = renderOpenedMultiSelect(onValueChange);

    pressArrowDown(input, 1); // (Select all) -> Chat
    expect(itemByText('Chat')).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onValueChange).not.toHaveBeenCalled();
    expect(document.querySelector('[cmdk-root]')).toBeNull();
    expect(screen.getByText('Chat')).toBeInTheDocument();
  });

  it('selects an unselected highlighted option on Enter and keeps the dropdown open', () => {
    const onValueChange = jest.fn();
    const { input, itemByText } = renderOpenedMultiSelect(onValueChange);

    pressArrowDown(input, 3); // (Select all) -> Chat -> VLM -> OCR
    expect(itemByText('OCR')).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onValueChange).toHaveBeenCalledWith(['chat', 'vlm', 'ocr']);
    expect(document.querySelector('[cmdk-root]')).not.toBeNull();
  });

  it('toggles all options on Enter while (Select all) is highlighted', () => {
    const onValueChange = jest.fn();
    const { input, items } = renderOpenedMultiSelect(onValueChange);

    // The first item is (Select all), auto-highlighted when the dropdown opens.
    expect(items()[0]).toHaveAttribute('aria-selected', 'true');

    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onValueChange).toHaveBeenCalledTimes(1);
    expect(onValueChange.mock.calls[0][0].slice().sort()).toEqual([
      'chat',
      'ocr',
      'vlm',
    ]);
    expect(document.querySelector('[cmdk-root]')).not.toBeNull();
  });

  it('deselects a selected option on click and keeps the dropdown open', () => {
    const onValueChange = jest.fn();
    const { itemByText } = renderOpenedMultiSelect(onValueChange);

    fireEvent.click(itemByText('Chat'));

    expect(onValueChange).toHaveBeenCalledWith(['vlm']);
    expect(document.querySelector('[cmdk-root]')).not.toBeNull();
  });
});
