import { fireEvent, render } from '@testing-library/react';
import NumberInput from '../number-input';

function getNumberInput(container: HTMLElement) {
  return container.querySelector('input[type="number"]') as HTMLInputElement;
}

function getStepButtons(container: HTMLElement) {
  const buttons = Array.from(
    container.querySelectorAll('button'),
  ) as HTMLButtonElement[];
  return { decrement: buttons[0], increment: buttons[1] };
}

describe('NumberInput', () => {
  describe('integer mode', () => {
    it('propagates the rounded integer while a fractional value is typed', () => {
      const onChange = jest.fn();
      const { container } = render(
        <NumberInput value={12} integer onChange={onChange} />,
      );
      const input = getNumberInput(container);

      fireEvent.change(input, { target: { value: '122.1' } });

      expect(input.value).toBe('122.1');
      expect(onChange).toHaveBeenCalledWith(122);
    });

    it('finalizes a fractional value to an integer on blur', () => {
      const onChange = jest.fn();
      const { container } = render(
        <NumberInput value={12} integer onChange={onChange} />,
      );
      const input = getNumberInput(container);

      fireEvent.change(input, { target: { value: '122.1' } });
      fireEvent.blur(input);

      expect(input.value).toBe('122');
      expect(onChange).toHaveBeenLastCalledWith(122);
    });

    it('clamps out-of-range values to max after rounding on blur', () => {
      const onChange = jest.fn();
      const { container } = render(
        <NumberInput value={3} min={0} max={8} integer onChange={onChange} />,
      );
      const input = getNumberInput(container);

      fireEvent.change(input, { target: { value: '31.1' } });
      fireEvent.blur(input);

      expect(input.value).toBe('8');
      expect(onChange).toHaveBeenLastCalledWith(8);
    });

    it('heals a fractional initial value by propagating the rounded integer', () => {
      const onChange = jest.fn();
      const { container } = render(
        <NumberInput value={12.5} integer onChange={onChange} />,
      );
      const input = getNumberInput(container);

      expect(input.value).toBe('13');
      expect(onChange).toHaveBeenCalledWith(13);
    });

    it('heals an out-of-range fractional initial value by clamping', () => {
      const onChange = jest.fn();
      const { container } = render(
        <NumberInput value={31.1} min={0} max={8} integer onChange={onChange} />,
      );
      const input = getNumberInput(container);

      expect(input.value).toBe('8');
      expect(onChange).toHaveBeenCalledWith(8);
    });

    it('steps from a rounded base value', () => {
      const onChange = jest.fn();
      const { container } = render(
        <NumberInput value={2} min={0} integer onChange={onChange} />,
      );
      const input = getNumberInput(container);
      const { increment, decrement } = getStepButtons(container);

      fireEvent.change(input, { target: { value: '2.6' } });
      fireEvent.click(increment);

      expect(input.value).toBe('4');
      expect(onChange).toHaveBeenLastCalledWith(4);

      fireEvent.click(decrement);

      expect(input.value).toBe('3');
      expect(onChange).toHaveBeenLastCalledWith(3);
    });
  });

  it('keeps propagating fractional values when integer mode is off', () => {
    const onChange = jest.fn();
    const { container } = render(
      <NumberInput value={1} max={5} onChange={onChange} />,
    );
    const input = getNumberInput(container);

    fireEvent.change(input, { target: { value: '4.8' } });
    fireEvent.blur(input);

    expect(input.value).toBe('4.8');
    expect(onChange).toHaveBeenLastCalledWith(4.8);
  });
});
