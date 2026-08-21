import { fireEvent, render, screen } from '@testing-library/react';
import React from 'react';

import NumberInput from '../number-input';

describe('NumberInput', () => {
  it('keeps the raw out-of-range value in the input but propagates the clamped value', () => {
    const onChange = jest.fn();
    const onRangeViolation = jest.fn();
    const { container } = render(
      <NumberInput
        value={512}
        min={0}
        max={128000}
        hideIcons
        onChange={onChange}
        onRangeViolation={onRangeViolation}
      />,
    );
    const input = container.querySelector('input[type=number]')!;

    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: '888888' } });

    expect(input).toHaveValue(888888);
    expect(onChange).toHaveBeenLastCalledWith(128000);
    expect(onRangeViolation).toHaveBeenLastCalledWith(true);
  });

  it('clears the range violation and normalizes the value on blur', () => {
    const onChange = jest.fn();
    const onRangeViolation = jest.fn();
    const { container } = render(
      <NumberInput
        value={512}
        min={0}
        max={128000}
        hideIcons
        onChange={onChange}
        onRangeViolation={onRangeViolation}
      />,
    );
    const input = container.querySelector('input[type=number]')!;

    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: '888888' } });
    fireEvent.blur(input);

    expect(input).toHaveValue(128000);
    expect(onChange).toHaveBeenLastCalledWith(128000);
    expect(onRangeViolation).toHaveBeenLastCalledWith(false);
  });

  it('propagates in-range values unchanged without a violation', () => {
    const onChange = jest.fn();
    const onRangeViolation = jest.fn();
    const { container } = render(
      <NumberInput
        value={512}
        min={0}
        max={128000}
        hideIcons
        onChange={onChange}
        onRangeViolation={onRangeViolation}
      />,
    );
    const input = container.querySelector('input[type=number]')!;

    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: '1024' } });

    expect(onChange).toHaveBeenLastCalledWith(1024);
    expect(onRangeViolation).toHaveBeenLastCalledWith(false);
  });

  it('keeps below-min intermediate edits in the display while propagating the minimum', () => {
    const onChange = jest.fn();
    const { container } = render(
      <NumberInput
        value={1024}
        min={512}
        max={128000}
        hideIcons
        onChange={onChange}
      />,
    );
    const input = container.querySelector('input[type=number]')!;

    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: '102' } });

    // Intermediate editing state: display keeps the raw value, but the form
    // receives the effective (clamped) value so sliders never go stale.
    expect(input).toHaveValue(102);
    expect(onChange).toHaveBeenLastCalledWith(512);

    fireEvent.blur(input);
    expect(input).toHaveValue(512);
    expect(onChange).toHaveBeenLastCalledWith(512);
  });

  it('does not overwrite the display with external value changes while focused', () => {
    function Harness() {
      const [value, setValue] = React.useState(512);
      return (
        <>
          <NumberInput
            value={value}
            min={0}
            max={128000}
            hideIcons
            onChange={setValue}
          />
          <button type="button" onClick={() => setValue(128000)}>
            external
          </button>
          <button type="button" onClick={() => setValue(64000)}>
            external-low
          </button>
        </>
      );
    }
    const { container } = render(<Harness />);
    const input = container.querySelector('input[type=number]')!;

    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: '888888' } });
    // External update (e.g. form echo of the clamped value) while editing.
    fireEvent.click(screen.getByText('external'));

    expect(input).toHaveValue(888888);

    // Once blurred, external/synced values show up again.
    fireEvent.blur(input);
    expect(input).toHaveValue(128000);

    // External changes while the input is not focused (e.g. dragging the
    // bound slider) still update the display.
    fireEvent.click(screen.getByText('external-low'));
    expect(input).toHaveValue(64000);
  });

  it('syncs the display back to the effective value when disabled', () => {
    function Harness() {
      const [value, setValue] = React.useState(512);
      const [enabled, setEnabled] = React.useState(true);
      return (
        <>
          <NumberInput
            value={value}
            min={0}
            max={128000}
            disabled={!enabled}
            hideIcons
            onChange={setValue}
          />
          <button type="button" onClick={() => setEnabled((e) => !e)}>
            toggle
          </button>
        </>
      );
    }
    const { container } = render(<Harness />);
    const input = container.querySelector('input[type=number]')!;

    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: '999999' } });
    fireEvent.click(screen.getByText('toggle'));

    expect(input).toBeDisabled();
    expect(input).toHaveValue(128000);
  });
});
