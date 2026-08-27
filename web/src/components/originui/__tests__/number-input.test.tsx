import NumberInput from '../number-input';
import { fireEvent, render } from '@testing-library/react';
import { useState } from 'react';

function Harness({
  step,
  initialValue = 0,
  min = 0,
  max = 1,
  hideIcons = true,
}: {
  step?: number;
  initialValue?: number;
  min?: number;
  max?: number;
  hideIcons?: boolean;
}) {
  const [value, setValue] = useState(initialValue);
  return (
    <NumberInput
      value={value}
      onChange={setValue}
      step={step}
      min={min}
      max={max}
      hideIcons={hideIcons}
    />
  );
}

const getButtons = () =>
  Array.from(document.querySelectorAll('button'));

const getInput = () => document.querySelector('input') as HTMLInputElement;

describe('NumberInput', () => {
  it('rounds typed decimals to the step precision', () => {
    render(<Harness step={0.01} />);
    const input = getInput();

    fireEvent.change(input, { target: { value: '0.3333' } });

    // The controlled round-trip feeds the rounded value back into the input.
    expect(input.value).toBe('0.33');
  });

  it('normalizes an over-precise persisted value to the step precision', () => {
    render(<Harness step={0.01} initialValue={0.3333333333333333} />);

    expect(getInput().value).toBe('0.33');
  });

  it('clamps and rounds on blur', () => {
    render(<Harness step={0.01} />);
    const input = getInput();

    // Out-of-range values stay raw while editing and are clamped on blur.
    fireEvent.change(input, { target: { value: '1.555' } });
    expect(input.value).toBe('1.555');

    fireEvent.blur(input);

    expect(input.value).toBe('1');
  });

  it('keeps full precision when no step is provided', () => {
    render(<Harness />);
    const input = getInput();

    fireEvent.change(input, { target: { value: '0.3333333333333333' } });

    expect(input.value).toBe('0.3333333333333333');
  });

  it('does not round decimals for whole-number steps', () => {
    render(<Harness step={1} />);
    const input = getInput();

    fireEvent.change(input, { target: { value: '0.5' } });

    expect(input.value).toBe('0.5');
  });

  it('derives precision from exponent-notation steps', () => {
    render(<Harness step={1e-7} />);
    const input = getInput();

    fireEvent.change(input, { target: { value: '0.3333333333333333' } });

    expect(input.value).toBe('0.3333333');
  });

  it('rounds to the fractional precision of steps greater than one', () => {
    render(<Harness step={1.5} max={10} />);
    const input = getInput();

    fireEvent.change(input, { target: { value: '1.234' } });

    expect(input.value).toBe('1.2');
  });

  it('steps by the configured step from the icon buttons', () => {
    render(<Harness step={0.01} initialValue={0.3} hideIcons={false} />);
    const input = getInput();
    const [decrement, increment] = getButtons();

    fireEvent.click(increment);
    expect(input.value).toBe('0.31');

    fireEvent.click(decrement);
    fireEvent.click(decrement);
    expect(input.value).toBe('0.29');
  });

  it('clamps icon-button stepping to the configured range', () => {
    render(<Harness step={0.01} initialValue={0} hideIcons={false} />);
    const input = getInput();
    const [decrement] = getButtons();

    // A full step below min would emit -0.01; the button stays at min.
    fireEvent.click(decrement);

    expect(input.value).toBe('0');
  });
});
