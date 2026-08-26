import NumberInput from '../number-input';
import { fireEvent, render } from '@testing-library/react';
import { useState } from 'react';

function Harness({
  step,
  initialValue = 0,
  min = 0,
  max = 1,
}: {
  step?: number;
  initialValue?: number;
  min?: number;
  max?: number;
}) {
  const [value, setValue] = useState(initialValue);
  return (
    <NumberInput
      value={value}
      onChange={setValue}
      step={step}
      min={min}
      max={max}
      hideIcons
    />
  );
}

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
});
