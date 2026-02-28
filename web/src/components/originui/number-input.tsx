import { isNumber, trim } from 'lodash';
import { MinusIcon, PlusIcon } from 'lucide-react';
import React, {
  FocusEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

interface NumberInputProps {
  className?: string;
  value?: number;
  onChange?: (value: number) => void;
  height?: number | string;
  min?: number;
  max?: number;
}

const NumberInput: React.FC<NumberInputProps> = ({
  className,
  value: initialValue,
  onChange,
  height,
  min = 0,
  max = Infinity,
}) => {
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
      if (newValue > max || newValue < min) {
        return;
      }
      setValue(newValue);
      onChange?.(newValue);
    }
  };

  const handleBlur: FocusEventHandler<HTMLInputElement> = useCallback(() => {
    if (isNumber(value)) {
      onChange?.(value);
    } else {
      const previousValue = valueRef.current ?? min;
      setValue(previousValue);
      onChange?.(previousValue);
    }
  }, [min, onChange, value]);

  const style = useMemo(
    () => ({
      height: height ? `${height.toString().replace('px', '')}px` : 'auto',
    }),
    [height],
  );
  return (
    <div
      className={`flex h-10 items-center space-x-2 border-[1px] rounded-lg w-[150px] ${className || ''}`}
      style={style}
    >
      <button
        type="button"
        className="w-10 p-2 focus:outline-none border-r-[1px]"
        onClick={handleDecrement}
        style={style}
      >
        <MinusIcon size={16} aria-hidden="true" />
      </button>
      <input
        type="text"
        value={value}
        onChange={handleChange}
        onBlur={handleBlur}
        className="w-full flex-1 text-center bg-transparent focus:outline-none"
        style={style}
        min={min}
      />
      <button
        type="button"
        className="w-10 p-2 focus:outline-none border-l-[1px]"
        onClick={handleIncrement}
        style={style}
      >
        <PlusIcon size={16} aria-hidden="true" />
      </button>
    </div>
  );
};

export default NumberInput;
