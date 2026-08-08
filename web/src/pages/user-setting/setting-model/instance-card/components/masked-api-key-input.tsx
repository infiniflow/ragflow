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

import { cn } from '@/lib/utils';
import { Eye, EyeOff } from 'lucide-react';
import { useState } from 'react';

/**
 * Fixed-length mask rendered while the key is hidden. Inside a password
 * input it produces a constant dot count, so the stored key's length is
 * not leaked through the UI.
 */
export const API_KEY_DISPLAY_MASK = '********';

interface MaskedApiKeyInputProps {
  value?: string;
  onChange?: (value: string) => void;
  onBlur?: () => void;
  name?: string;
  placeholder?: string;
  disabled?: boolean;
}

/**
 * Password input for an already-stored API key.
 *
 * - Hidden (default): renders a fixed-length mask regardless of the
 *   key's real length.
 * - Revealed (eye toggle): renders the real stored key, editable.
 * - Focusing the masked field starts a fresh entry: the first keystroke
 *   replaces the stored key; focusing out without typing keeps it.
 */
export function MaskedApiKeyInput({
  value = '',
  onChange,
  onBlur,
  name,
  placeholder,
  disabled,
}: MaskedApiKeyInputProps) {
  const [show, setShow] = useState(false);
  // Non-null while the user is typing a replacement key into the masked
  // field; back to null restores the fixed-mask display.
  const [draft, setDraft] = useState<string | null>(null);

  const displayValue =
    draft !== null ? draft : show || !value ? value : API_KEY_DISPLAY_MASK;

  return (
    <div className="relative w-full">
      <input
        name={name}
        type={show ? 'text' : 'password'}
        className={cn(
          'flex h-8 w-full rounded-md border-0.5 border-border-button bg-bg-input px-3 py-2 outline-none text-sm text-text-primary',
          'placeholder:text-text-disabled',
          'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-primary',
          'disabled:cursor-not-allowed disabled:opacity-50 transition-colors',
        )}
        style={{ paddingInlineEnd: '40px' }}
        value={displayValue}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        onFocus={() => {
          // Start a fresh entry so the first keystroke replaces the
          // stored key instead of appending to the mask.
          if (!show && draft === null && value) {
            setDraft('');
          }
        }}
        onChange={(e) => {
          setDraft(e.target.value);
          onChange?.(e.target.value);
        }}
        onBlur={() => {
          setDraft(null);
          onBlur?.();
        }}
      />
      <button
        type="button"
        tabIndex={-1}
        className="p-2 text-text-secondary absolute border-0 end-1 top-[50%] translate-y-[-50%]"
        onClick={() => {
          setShow(!show);
          setDraft(null);
        }}
      >
        {show ? (
          <EyeOff className="size-[1em]" />
        ) : (
          <Eye className="size-[1em]" />
        )}
      </button>
    </div>
  );
}
