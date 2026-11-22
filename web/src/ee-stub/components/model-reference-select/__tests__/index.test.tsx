import { fireEvent, render, screen } from '@testing-library/react';
import { createRef, forwardRef } from 'react';
import { ModelReferenceSelect } from '../index';

const mockDirectSelectProps = jest.fn();

const FakeDirectSelect = forwardRef<
  HTMLButtonElement,
  import('@/components/model-tree-select').ModelTreeSelectProps
>(function FakeDirectSelect(props, ref) {
  mockDirectSelectProps(props);
  const handleClick = () => props.onChange?.('next-model');
  return (
    <button
      ref={ref}
      type="button"
      data-testid="direct-select"
      onClick={handleClick}
    >
      {props.value}
    </button>
  );
});

describe('OSS ModelReferenceSelect', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders only the direct selector and preserves its value and events', () => {
    const onChange = jest.fn();
    const ref = createRef<HTMLButtonElement>();
    render(
      <ModelReferenceSelect
        ref={ref}
        directModelSelect={FakeDirectSelect}
        value="model-1"
        onChange={onChange}
        placeholder="Pick a model"
        allowClear
      />,
    );

    const direct = screen.getByTestId('direct-select');
    expect(direct).toHaveTextContent('model-1');
    expect(ref.current).toBe(direct);
    expect(screen.queryByRole('radio')).not.toBeInTheDocument();
    expect(mockDirectSelectProps).toHaveBeenCalledTimes(1);
    expect(mockDirectSelectProps.mock.lastCall?.[0]).toMatchObject({
      value: 'model-1',
      placeholder: 'Pick a model',
      allowClear: true,
    });
    expect(mockDirectSelectProps.mock.lastCall?.[0]).not.toHaveProperty(
      'directModelSelect',
    );

    fireEvent.click(direct);
    expect(onChange).toHaveBeenCalledWith('next-model');
  });
});
