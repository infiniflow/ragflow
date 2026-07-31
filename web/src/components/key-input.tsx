import { Input, InputProps } from '@/components/ui/input';
import { ChangeEvent, forwardRef, useCallback } from 'react';

type KeyInputProps = {
  value?: string;
  onChange?: (value: string) => void;
  searchValue?: string | RegExp;
} & Omit<InputProps, 'onChange'>;

export const KeyInput = forwardRef<HTMLInputElement, KeyInputProps>(
  function KeyInput(
    { value, onChange, searchValue = /[^a-zA-Z0-9_]/g, ...props },
    ref,
  ) {
    const handleChange = useCallback(
      (e: ChangeEvent<HTMLInputElement>) => {
        const value = e.target.value ?? '';
        const filteredValue = value.replace(searchValue, '');
        onChange?.(filteredValue);
      },
      [onChange, searchValue],
    );

    return <Input {...props} value={value} onChange={handleChange} ref={ref} />;
  },
);
