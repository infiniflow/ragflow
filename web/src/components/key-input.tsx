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
