import { Input } from '@/components/ui/input';
import { PenLine } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useHandleNameChange } from './use-handle-name-change';

type NameInputProps = {
  value: string;
  onChange: (value: string) => void;
};

export function NameInput({ value, onChange }: NameInputProps) {
  const { name, handleNameBlur, handleNameChange } = useHandleNameChange(value);
  const inputRef = useRef<HTMLInputElement>(null);

  const [isEditingMode, setIsEditingMode] = useState(false);

  const switchIsEditingMode = useCallback(() => {
    setIsEditingMode((prev) => !prev);
  }, []);

  const handleBlur = useCallback(() => {
    const nextName = handleNameBlur();
    setIsEditingMode(false);
    onChange(nextName);
  }, [handleNameBlur, onChange]);

  useEffect(() => {
    if (isEditingMode && inputRef.current) {
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [isEditingMode]);

  return (
    <div className="flex items-center gap-1 flex-1">
      {isEditingMode ? (
        <Input
          ref={inputRef}
          value={name}
          onBlur={handleBlur}
          onChange={handleNameChange}
        ></Input>
      ) : (
        <div className="flex items-center justify-between gap-2 text-base w-full">
          <span className="truncate flex-1">{name}</span>
          <PenLine
            onClick={switchIsEditingMode}
            className="size-3.5 text-text-secondary cursor-pointer hidden group-hover:block"
          />
        </div>
      )}
    </div>
  );
}
