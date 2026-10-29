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

import { Input } from '@/components/ui/input';
import { forwardRef } from 'react';
import type { ComponentProps, FocusEvent, MouseEvent } from 'react';

export const SAVED_CREDENTIAL_MASK = '********';

type SavedCredentialInputProps = Omit<
  ComponentProps<typeof Input>,
  'type' | 'value' | 'onFocus' | 'onMouseUp'
> & {
  maskedValue?: string;
  value?: unknown;
};

export const SavedCredentialInput = forwardRef<
  HTMLInputElement,
  SavedCredentialInputProps
>(function SavedCredentialInput({ maskedValue, value, ...inputProps }, ref) {
  const isStoredCredential = value === SAVED_CREDENTIAL_MASK;
  const displayValue = isStoredCredential
    ? maskedValue || SAVED_CREDENTIAL_MASK
    : typeof value === 'string'
      ? value
      : '';
  const handleFocus = (event: FocusEvent<HTMLInputElement>) => {
    if (isStoredCredential) {
      event.currentTarget.select();
    }
  };
  const handleMouseUp = (event: MouseEvent<HTMLInputElement>) => {
    if (isStoredCredential) {
      event.preventDefault();
      event.currentTarget.select();
    }
  };

  return (
    <Input
      ref={ref}
      {...inputProps}
      type={isStoredCredential ? 'text' : 'password'}
      value={displayValue}
      onFocus={handleFocus}
      onMouseUp={handleMouseUp}
    />
  );
});

SavedCredentialInput.displayName = 'SavedCredentialInput';
