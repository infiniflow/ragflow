/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
