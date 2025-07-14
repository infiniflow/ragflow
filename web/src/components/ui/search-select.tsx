import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { Check, ChevronDown } from 'lucide-react';
import * as React from 'react';

export interface SearchableSelectProps {
  options: Array<{ value: string; label: string }>;
  placeholder?: string;
  value?: string;
  onChange?: (value: string) => void;
  className?: string;
  disabled?: boolean;
}

export const SearchableSelect = React.forwardRef<
  HTMLDivElement,
  SearchableSelectProps
>(
  (
    {
      options,
      placeholder = 'Please select...',
      value,
      onChange,
      className,
      disabled,
    },
    ref,
  ) => {
    const [open, setOpen] = React.useState(false);
    const [searchValue, setSearchValue] = React.useState('');
    const inputRef = React.useRef<HTMLInputElement>(null);

    // Sync external value changes
    const [selectedValue, setSelectedValue] = React.useState(value || '');
    React.useEffect(() => {
      if (value !== undefined) {
        setSelectedValue(value);
      }
    }, [value]);

    // Filter options based on search value
    const filteredOptions = React.useMemo(() => {
      if (!searchValue.trim()) return options;
      return options.filter((option) =>
        option.label.toLowerCase().includes(searchValue.toLowerCase().trim()),
      );
    }, [options, searchValue]);

    // Handle option selection
    const handleSelect = React.useCallback(
      (value: string) => {
        setSelectedValue(value);
        onChange?.(value);
        setOpen(false);
        inputRef.current?.focus();
      },
      [onChange],
    );

    // Get selected label
    const selectedLabel = React.useMemo(() => {
      if (selectedValue) {
        const option = options.find((opt) => opt.value === selectedValue);
        return option?.label || '';
      }
      return '';
    }, [selectedValue, options]);

    // Reset search when popover closes
    React.useEffect(() => {
      if (!open) {
        setSearchValue('');
      }
    }, [open]);

    return (
      <Popover open={open} onOpenChange={setOpen} modal>
        <PopoverTrigger asChild>
          <div
            ref={ref}
            className={cn(
              'flex items-center justify-between h-8 px-3 border rounded-md bg-background text-foreground cursor-pointer select-none transition-all duration-200 focus-within:ring-2 focus-within:ring-primary/50 focus-within:border-primary',
              disabled && 'opacity-50 cursor-not-allowed',
              className,
            )}
            onClick={(e) => {
              if (disabled) {
                e.preventDefault();
                return;
              }
              setOpen(!open);
            }}
            aria-disabled={disabled}
          >
            <div className="text-base">
              {selectedValue ? selectedLabel : searchValue}
            </div>
            <ChevronDown
              className={cn(
                'h-4 w-4 opacity-50 transition-transform duration-200',
                open && 'rotate-180',
              )}
            />
          </div>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="z-50 w-[--radix-popover-trigger-width] rounded-md border bg-popover p-0 shadow-md"
          style={{ minWidth: 'fit-content' }}
        >
          <input
            type="text"
            value={searchValue}
            onChange={(e) => {
              setSearchValue(e.target.value);
            }}
            placeholder={placeholder}
            className="w-full border-b px-3 py-2 text-sm focus:outline-none bg-transparent"
            autoFocus
          />
          <ul className="py-1 max-h-60 overflow-auto">
            {filteredOptions.length > 0 ? (
              filteredOptions.map((option) => (
                <li
                  key={option.value}
                  onClick={() => handleSelect(option.value)}
                  className={cn(
                    'relative flex cursor-default select-none items-center px-3 py-2 text-sm hover:bg-accent hover:text-accent-foreground',
                    selectedValue === option.value && 'bg-accent font-medium',
                  )}
                >
                  {option.label}
                  {selectedValue === option.value && (
                    <Check className="ml-auto h-4 w-4" />
                  )}
                </li>
              ))
            ) : (
              <li className="px-3 py-2 text-sm text-muted-foreground">
                No matching options found
              </li>
            )}
          </ul>
        </PopoverContent>
      </Popover>
    );
  },
);

SearchableSelect.displayName = 'SearchableSelect';
