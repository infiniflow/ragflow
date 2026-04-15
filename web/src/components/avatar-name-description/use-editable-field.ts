'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

interface UseEditableFieldOptions {
  required?: boolean;
}

interface UseEditableFieldReturn {
  isEditing: boolean;
  inputRef: React.RefObject<HTMLInputElement | null>;
  previousValueRef: React.RefObject<string>;
  handleEnterEdit: (currentValue: string) => void;
  handleExitEdit: () => void;
  handleKeyDown: (e: React.KeyboardEvent<HTMLInputElement>) => void;
  handleBlur: (currentValue: string, onChange: (value: string) => void) => void;
}

export function useEditableField(
  options: UseEditableFieldOptions = {},
): UseEditableFieldReturn {
  const { required = true } = options;
  const [isEditing, setIsEditing] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const previousValueRef = useRef<string>('');

  // Auto-focus when entering edit mode
  useEffect(() => {
    if (isEditing) {
      const frameId = requestAnimationFrame(() => {
        inputRef.current?.focus();
      });

      return () => cancelAnimationFrame(frameId);
    }
  }, [isEditing]);

  const handleEnterEdit = useCallback((currentValue: string) => {
    previousValueRef.current = currentValue;
    setIsEditing(true);
  }, []);

  const handleExitEdit = useCallback(() => {
    setIsEditing(false);
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        setIsEditing(false);
      }
      if (e.key === 'Escape') {
        setIsEditing(false);
      }
    },
    [],
  );

  const handleBlur = useCallback(
    (currentValue: string, onChange: (value: string) => void) => {
      // If required and value is empty, restore to previous value
      if (required && !currentValue?.trim()) {
        onChange(previousValueRef.current);
      }
      setIsEditing(false);
    },
    [required],
  );

  return {
    isEditing,
    inputRef,
    previousValueRef,
    handleEnterEdit,
    handleExitEdit,
    handleKeyDown,
    handleBlur,
  };
}
