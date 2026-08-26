import { cn } from '@/lib/utils';
import { isNumber, omit, trim } from 'lodash';
import { MinusIcon, PlusIcon } from 'lucide-react';
import React, {
  FocusEventHandler,
  forwardRef,
  KeyboardEventHandler,
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
  step?: number;
  hideIcons?: boolean;
  integer?: boolean;
  inputClassName?: string;
}

// Keys that would introduce a fractional part or exponent notation in
// integer mode; blocked on keydown so a decimal point can never be typed.
const BlockedIntegerKeys = ['.', 'e', 'E', '+'];

// Decimal places implied by a fractional step (e.g. 0.01 -> 2, 0.1 -> 1).
// Undefined or whole-number steps keep the value untouched so integer
// sliders are not affected.
const getStepDecimals = (step?: number): number => {
  if (!step || step >= 1) {
    return 0;
  }
  return String(step).split('.')[1]?.length ?? 0;
};

const roundToStepPrecision = (value: number, step?: number): number => {
  const decimals = getStepDecimals(step);
  return decimals > 0 ? Number(value.toFixed(decimals)) : value;
};

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
    step,
    hideIcons = false,
    integer = false,
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
      // Normalize over-precise persisted values (e.g. a filename embedding
      // weight stored as 0.3333333333333333) to the step precision.
      setValue(roundToStepPrecision(initialValue, step));
    }
  }, [initialValue, step]);

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

  const handleKeyDown: KeyboardEventHandler<HTMLInputElement> = (e) => {
    if (
      integer &&
      (BlockedIntegerKeys.includes(e.key) || (e.key === '-' && min >= 0))
    ) {
      e.preventDefault();
    }
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
      // Pasted decimals bypass the keydown guard; reject them in integer
      // mode instead of rounding silently.
      if (integer && !Number.isInteger(newValue)) {
        return;
      }
      // Show the raw typed value as-is, even when it falls outside [min, max]
      // (e.g. deleting "1024" → "102" when min=512), so the controlled input
      // never snaps back mid-edit. Values leaving the component are rounded
      // to the step precision so the form never holds more decimals than the
      // step allows; out-of-range values are not propagated at all and
      // handleBlur clamps them into range on focus loss.
      setValue(newValue);
      if (newValue >= min && newValue <= max) {
        onChange?.(roundToStepPrecision(newValue, step));
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
        finalValue = roundToStepPrecision(finalValue, step);
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
        finalValue = roundToStepPrecision(finalValue, step);
        setValue(finalValue);
        onChange?.(finalValue);
      }
      // Keep the caller's blur notification (e.g. react-hook-form's
      // field.onBlur) alive — it is destructured out of props so that the
      // spread below cannot silently replace this handler.
      onBlurProp?.(e);
    },
    [min, max, step, onChange, onBlurProp, value],
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
          onKeyDown={handleKeyDown}
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
          step={step}
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
