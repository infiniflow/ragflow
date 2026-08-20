import { cn } from '@/lib/utils';
import { isNumber, omit, trim } from 'lodash';
import { MinusIcon, PlusIcon } from 'lucide-react';
import React, {
  FocusEventHandler,
  forwardRef,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { InputProps } from '../ui/input';

interface NumberInputProps {
  className?: string;
  value?: number;
  onChange?: (value: number) => void;
  height?: number | string;
  min?: number;
  max?: number;
  hideIcons?: boolean;
  inputClassName?: string;
}

const NumberInput = forwardRef<
  HTMLInputElement,
  Omit<InputProps, 'onChange' | 'value'> & NumberInputProps
>(function NumberInput(
  {
    className,
    value: initialValue,
    onChange,
    onBlur: onBlurProp,
    height,
    min = 0,
    max = Infinity,
    hideIcons = false,
    inputClassName,
    ...props
  },
  ref,
) {
  const [value, setValue] = useState<number | ''>(() => {
    return initialValue ?? 0;
  });

  const valueRef = useRef<number>();

  useEffect(() => {
    if (initialValue !== undefined) {
      setValue(initialValue);
    }
  }, [initialValue]);

  const handleDecrement = () => {
    if (isNumber(value) && value > min) {
      setValue(value - 1);
      onChange?.(value - 1);
    }
  };

  const handleIncrement = () => {
    if (!isNumber(value)) {
      return;
    }
    if (value > max - 1) {
      return;
    }
    setValue(value + 1);
    onChange?.(value + 1);
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const currentValue = e.target.value;
    const newValue = Number(currentValue);

    if (trim(currentValue) === '') {
      if (isNumber(value)) {
        valueRef.current = value;
      }
      setValue('');
      return;
    }

    if (!isNaN(newValue)) {
      // Show the raw typed value as-is, even when it falls outside [min, max]
      // (e.g. deleting "1024" → "102" when min=512), so the controlled input
      // never snaps back mid-edit. Out-of-range values are not propagated to
      // the form; handleBlur clamps them into range on focus loss.
      setValue(newValue);
      if (newValue >= min && newValue <= max) {
        onChange?.(newValue);
      }
    }
  };

  const handleBlur: FocusEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      if (isNumber(value)) {
        let finalValue = value;
        if (value < min) {
          finalValue = min;
        } else if (value > max) {
          finalValue = max;
        }
        if (finalValue !== value) {
          setValue(finalValue);
        }
        onChange?.(finalValue);
      } else {
        const previousValue = valueRef.current ?? min;
        let finalValue = previousValue;
        if (previousValue < min) {
          finalValue = min;
        } else if (previousValue > max) {
          finalValue = max;
        }
        setValue(finalValue);
        onChange?.(finalValue);
      }
      // Keep the caller's blur notification (e.g. react-hook-form's
      // field.onBlur) alive — it is destructured out of props so that the
      // spread below cannot silently replace this handler.
      onBlurProp?.(e);
    },
    [min, max, onChange, onBlurProp, value],
  );

  const style = useMemo(
    () => ({
      height: height ? `${height.toString().replace('px', '')}px` : 'auto',
    }),
    [height],
  );
  return (
    <>
      <style>{`
        .number-input-hide-spin::-webkit-inner-spin-button,
        .number-input-hide-spin::-webkit-outer-spin-button {
          -webkit-appearance: none;
          margin: 0;
        }
        .number-input-hide-spin[type='number'] {
          -moz-appearance: textfield;
        }
      `}</style>
      <div
        className={cn(
          `flex h-10 items-center space-x-2 border-[1px] rounded-lg w-[150px]`,
          className,
        )}
        style={style}
        ref={ref}
      >
        {hideIcons || (
          <button
            type="button"
            className="w-10 p-2 focus:outline-none border-r-[1px]"
            onClick={handleDecrement}
            style={style}
          >
            <MinusIcon size={16} aria-hidden="true" />
          </button>
        )}
        <input
          type="number"
          value={value}
          onChange={handleChange}
          onBlur={handleBlur}
          className={cn(
            'w-full flex-1 text-center bg-transparent focus-visible:outline-none number-input-hide-spin',
            'disabled:cursor-not-allowed disabled:opacity-50 transition-colors',
            {
              'focus-visible:ring-1 focus-visible:ring-accent-primary rounded-lg':
                hideIcons,
            },
            inputClassName,
          )}
          style={style}
          min={min}
          {...omit(props, ['prefix', 'suffix'])}
        />
        {hideIcons || (
          <button
            type="button"
            className="w-10 p-2 focus:outline-none border-l-[1px]"
            onClick={handleIncrement}
            style={style}
          >
            <PlusIcon size={16} aria-hidden="true" />
          </button>
        )}
      </div>
    </>
  );
});

export default NumberInput;
