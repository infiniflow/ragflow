import { Input } from '@/components/ui/input';
import { ChangeEvent, useCallback } from 'react';

type KeyInputProps = {
  value?: string;
  onChange?: (value: string) => void;
  searchValue?: string | RegExp;
};

export function KeyInput({
  value,
  onChange,
  searchValue = /[^a-zA-Z0-9_]/g,
}: KeyInputProps) {
  const handleChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value ?? '';
      const filteredValue = value.replace(searchValue, '');
      onChange?.(filteredValue);
    },
    [onChange, searchValue],
  );

  return <Input value={value} onChange={handleChange} />;
}
