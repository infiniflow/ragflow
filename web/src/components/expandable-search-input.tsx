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

import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { Search, X } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface ExpandableSearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  inputClassName?: string;
  width?: number | string;
}

export function ExpandableSearchInput({
  value,
  onChange,
  placeholder,
  className,
  inputClassName,
  width = 192,
}: ExpandableSearchInputProps) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);

  const handleToggle = useCallback(() => {
    setIsOpen((prev) => {
      const next = !prev;
      if (!next) {
        onChange('');
      }
      return next;
    });
  }, [onChange]);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(e.target.value);
    },
    [onChange],
  );

  return (
    <div className={cn('relative flex items-center gap-2', className)}>
      <div
        className={cn(
          'transition-all duration-300 ease-in-out',
          isOpen ? 'opacity-100' : 'w-0 overflow-hidden opacity-0',
        )}
        style={{ width: isOpen ? width : 0 }}
      >
        <SearchInput
          value={value}
          onChange={handleChange}
          className={cn('w-full', inputClassName)}
          autoFocus={isOpen}
          placeholder={placeholder}
          suffix={
            isOpen ? (
              <button
                type="button"
                onClick={handleToggle}
                className="p-1 text-text-secondary hover:text-text-primary"
                aria-label={t('common.close', 'Close')}
              >
                <X className="h-4 w-4" />
              </button>
            ) : null
          }
        />
      </div>
      {!isOpen && (
        <Button
          variant="ghost"
          size="icon"
          type="button"
          onClick={handleToggle}
          aria-label={t('common.search')}
        >
          <Search className="h-5 w-5" />
        </Button>
      )}
    </div>
  );
}
