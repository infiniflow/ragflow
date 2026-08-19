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
  onRangeViolation?: (outOfRange: boolean) => void;
}

const NumberInput = forwardRef<
  HTMLInputElement,
  Omit<InputProps, 'onChange' | 'value'> & NumberInputProps
>(function NumberInput(
  {
    className,
    value: initialValue,
    onChange,
    height,
    min = 0,
    max = Infinity,
    hideIcons = false,
    inputClassName,
    onRangeViolation,
    disabled,
    ...props
  },
  ref,
) {
  const [value, setValue] = useState<number | ''>(() => {
    return initialValue ?? 0;
  });

  const isFocusedRef = useRef(false);

  const valueRef = useRef<number>();

  const clamp = useCallback(
    (v: number) => Math.min(Math.max(v, min), max),
    [min, max],
  );

  useEffect(() => {
    if (initialValue !== undefined) {
      setValue((prev) => {
        // Keep the raw typed value while the input is focused: the effective
        // (clamped) value has already been propagated via onChange, and the
        // display is normalized back into the range on blur. The focus flag
        // lives in a ref so the transition itself does not re-trigger this
        // sync with a stale initialValue.
        if (isFocusedRef.current) {
          return prev;
        }
        return initialValue;
      });
    }
  }, [initialValue]);

  useEffect(() => {
    // A disabled input cannot be edited, so there is no draft to protect:
    // drop any stale out-of-range display value and show the effective one.
    if (disabled && initialValue !== undefined) {
      setValue(initialValue);
      isFocusedRef.current = false;
    }
  }, [disabled, initialValue]);

  const handleDecrement = () => {
    if (isNumber(value) && value > min) {
      const nextValue = clamp(value - 1);
      setValue(nextValue);
      onChange?.(nextValue);
      onRangeViolation?.(false);
    }
  };

  const handleIncrement = () => {
    if (!isNumber(value)) {
      return;
    }
    const nextValue = clamp(value + 1);
    if (nextValue <= value) {
      return;
    }
    setValue(nextValue);
    onChange?.(nextValue);
    onRangeViolation?.(false);
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const currentValue = e.target.value;
    const newValue = Number(currentValue);

    if (trim(currentValue) === '') {
      if (isNumber(value)) {
        valueRef.current = value;
      }
      onRangeViolation?.(false);
      setValue('');
      return;
    }

    if (!isNaN(newValue)) {
      // Allow intermediate editing states that fall outside [min, max]
      // (e.g. deleting "1024" → "102" when min=512): the raw value stays in
      // the input, while the clamped value is propagated immediately so
      // bound UI (e.g. sliders) never shows a stale position. The input is
      // normalized back into the range on blur.
      onRangeViolation?.(newValue < min || newValue > max);
      setValue(newValue);
      onChange?.(clamp(newValue));
    }
  };

  const handleBlur: FocusEventHandler<HTMLInputElement> = useCallback(() => {
    isFocusedRef.current = false;
    if (isNumber(value)) {
      const finalValue = clamp(value);
      if (finalValue !== value) {
        setValue(finalValue);
      }
      onRangeViolation?.(false);
      onChange?.(finalValue);
    } else {
      const previousValue = valueRef.current ?? min;
      const finalValue = clamp(previousValue);
      setValue(finalValue);
      onRangeViolation?.(false);
      onChange?.(finalValue);
    }
  }, [min, onChange, onRangeViolation, value, clamp]);

  const handleFocus = useCallback(() => {
    isFocusedRef.current = true;
  }, []);

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
          {...omit(props, ['prefix', 'suffix'])}
          type="number"
          value={value}
          disabled={disabled}
          onChange={handleChange}
          onBlur={handleBlur}
          onFocus={handleFocus}
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
