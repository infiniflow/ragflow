import { MinusIcon, PlusIcon } from 'lucide-react';
import React, { useEffect, useMemo, useState } from 'react';

interface NumberInputProps {
  className?: string;
  value?: number;
  onChange?: (value: number) => void;
  height?: number | string;
  min?: number;
}

const NumberInput: React.FC<NumberInputProps> = ({
  className,
  value: initialValue,
  onChange,
  height,
  min = 0,
}) => {
  const [value, setValue] = useState<number>(() => {
    return initialValue ?? 0;
  });

  useEffect(() => {
    if (initialValue !== undefined) {
      setValue(initialValue);
    }
  }, [initialValue]);

  const handleDecrement = () => {
    if (value > 0) {
      setValue(value - 1);
      onChange?.(value - 1);
    }
  };

  const handleIncrement = () => {
    setValue(value + 1);
    onChange?.(value + 1);
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = Number(e.target.value);
    if (!isNaN(newValue)) {
      setValue(newValue);
      onChange?.(newValue);
    }
  };

  const handleInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    // If the input value is not a number, the input is not allowed
    if (!/^\d*$/.test(e.target.value)) {
      e.preventDefault();
    }
  };
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
        onInput={handleInput}
        onChange={handleChange}
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
