import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { X } from 'lucide-react';
import * as React from 'react';
import { useTranslation } from 'react-i18next';
import { Popover, PopoverContent, PopoverTrigger } from './popover';

/** Interface for tag select options */
export interface InputSelectOption {
  /** Value of the option */
  value: string;
  /** Display label of the option */
  label: string;
}

/** Properties for the InputSelect component */
export interface InputSelectProps {
  /** Options for the select component */
  options?: InputSelectOption[];
  /** Selected values - type depends on the input type */
  value?: string | string[] | number | number[] | Date | Date[];
  /** Callback when value changes */
  onChange?: (
    value: string | string[] | number | number[] | Date | Date[],
  ) => void;
  /** Placeholder text */
  placeholder?: string;
  /** Additional class names */
  className?: string;
  /** Style object */
  style?: React.CSSProperties;
  /** Whether to allow multiple selections */
  multi?: boolean;
  /** Type of input: text, number, date, or datetime */
  type?: 'text' | 'number' | 'date' | 'datetime';
}

const InputSelect = React.forwardRef<HTMLInputElement, InputSelectProps>(
  (
    {
      options = [],
      value = [],
      onChange,
      placeholder = 'Select tags...',
      className,
      style,
      multi = false,
      type = 'text',
    },
    ref,
  ) => {
    const [inputValue, setInputValue] = React.useState('');
    const [open, setOpen] = React.useState(false);
    const [isFocused, setIsFocused] = React.useState(false);
    const inputRef = React.useRef<HTMLInputElement>(null);
    const { t } = useTranslation();

    // Normalize value to array for consistent handling based on type
    const normalizedValue = React.useMemo(() => {
      if (Array.isArray(value)) {
        return value;
      } else if (value !== undefined && value !== null) {
        if (type === 'number') {
          return typeof value === 'number' ? [value] : [Number(value)];
        } else if (type === 'date' || type === 'datetime') {
          return value instanceof Date ? [value] : [new Date(value as any)];
        } else {
          return typeof value === 'string' ? [value] : [String(value)];
        }
      } else {
        return [];
      }
    }, [value, type]);

    /**
     * Removes a tag from the selected values
     * @param tagValue - The value of the tag to remove
     */
    const handleRemoveTag = (tagValue: any) => {
      let newValue: any[];

      if (type === 'number') {
        newValue = (normalizedValue as number[]).filter((v) => v !== tagValue);
      } else if (type === 'date' || type === 'datetime') {
        newValue = (normalizedValue as Date[]).filter(
          (v) => v.getTime() !== tagValue.getTime(),
        );
      } else {
        newValue = (normalizedValue as string[]).filter((v) => v !== tagValue);
      }

      // Return single value if not multi-select, otherwise return array
      let result: string | number | Date | string[] | number[] | Date[];
      if (multi) {
        result = newValue;
      } else {
        if (type === 'number') {
          result = newValue[0] || 0;
        } else if (type === 'date' || type === 'datetime') {
          result = newValue[0] || new Date();
        } else {
          result = newValue[0] || '';
        }
      }

      onChange?.(result);
    };

    /**
     * Adds a tag to the selected values
     * @param optionValue - The value of the tag to add
     */
    const handleAddTag = (optionValue: any) => {
      let newValue: any[];

      if (multi) {
        // For multi-select, add to array if not already included
        if (type === 'number') {
          const numValue =
            typeof optionValue === 'number' ? optionValue : Number(optionValue);
          if (
            !(normalizedValue as number[]).includes(numValue) &&
            !isNaN(numValue)
          ) {
            newValue = [...(normalizedValue as number[]), numValue];
            onChange?.(newValue as number[]);
          }
        } else if (type === 'date' || type === 'datetime') {
          const dateValue =
            optionValue instanceof Date ? optionValue : new Date(optionValue);
          if (
            !(normalizedValue as Date[]).some(
              (d) => d.getTime() === dateValue.getTime(),
            )
          ) {
            newValue = [...(normalizedValue as Date[]), dateValue];
            onChange?.(newValue as Date[]);
          }
        } else {
          if (!(normalizedValue as string[]).includes(optionValue)) {
            newValue = [...(normalizedValue as string[]), optionValue];
            onChange?.(newValue as string[]);
          }
        }
      } else {
        // For single-select, replace the value
        if (type === 'number') {
          const numValue =
            typeof optionValue === 'number' ? optionValue : Number(optionValue);
          if (!isNaN(numValue)) {
            onChange?.(numValue);
          }
        } else if (type === 'date' || type === 'datetime') {
          const dateValue =
            optionValue instanceof Date ? optionValue : new Date(optionValue);
          onChange?.(dateValue);
        } else {
          onChange?.(optionValue);
        }
      }

      setInputValue('');
      setOpen(false); // Close the popover after adding a tag
    };

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      setInputValue(newValue);
      setOpen(!!newValue); // Open popover when there's input
    };

    const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (
        e.key === 'Backspace' &&
        inputValue === '' &&
        normalizedValue.length > 0
      ) {
        // Remove last tag when pressing backspace on empty input
        const newValue = [...normalizedValue];
        newValue.pop();
        // Return single value if not multi-select, otherwise return array
        let result: string | number | Date | string[] | number[] | Date[];
        if (multi) {
          result = newValue;
        } else {
          if (type === 'number') {
            result = newValue[0] || 0;
          } else if (type === 'date' || type === 'datetime') {
            result = newValue[0] || new Date();
          } else {
            result = newValue[0] || '';
          }
        }

        onChange?.(result);
      } else if (e.key === 'Enter' && inputValue.trim() !== '') {
        e.preventDefault();

        let valueToAdd: any;

        if (type === 'number') {
          const numValue = Number(inputValue);
          if (isNaN(numValue)) return; // Don't add invalid numbers
          valueToAdd = numValue;
        } else if (type === 'date' || type === 'datetime') {
          const dateValue = new Date(inputValue);
          if (isNaN(dateValue.getTime())) return; // Don't add invalid dates
          valueToAdd = dateValue;
        } else {
          valueToAdd = inputValue;
        }

        // Add input value as a new tag if it doesn't exist in options
        const matchedOption = options.find(
          (opt) => opt.label.toLowerCase() === inputValue.toLowerCase(),
        );

        if (matchedOption) {
          handleAddTag(matchedOption.value);
        } else {
          // If not in options, create a new tag with the input value
          if (
            !normalizedValue.some((v) =>
              type === 'number'
                ? Number(v) === Number(valueToAdd)
                : type === 'date' || type === 'datetime'
                  ? new Date(v as any).getTime() === valueToAdd.getTime()
                  : String(v) === valueToAdd,
            ) &&
            inputValue.trim() !== ''
          ) {
            handleAddTag(valueToAdd);
          }
        }
      } else if (e.key === 'Escape') {
        inputRef.current?.blur();
        setOpen(false);
      } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        // Allow navigation in the dropdown
        return;
      }
    };

    const handleContainerClick = () => {
      inputRef.current?.focus();
      setOpen(true);
      setIsFocused(true);
    };

    const handleInputFocus = () => {
      setOpen(true);
      setIsFocused(true);
    };

    const handleInputBlur = () => {
      // Delay closing to allow click on options
      setTimeout(() => {
        setOpen(false);
        setIsFocused(false);
      }, 150);
    };

    // Filter options to exclude already selected ones (only for multi-select)
    const availableOptions = multi
      ? options.filter(
          (option) =>
            !normalizedValue.some((v) =>
              type === 'number'
                ? Number(v) === Number(option.value)
                : type === 'date' || type === 'datetime'
                  ? new Date(v as any).getTime() ===
                    new Date(option.value).getTime()
                  : String(v) === option.value,
            ),
        )
      : options;

    const filteredOptions = availableOptions.filter(
      (option) =>
        !inputValue ||
        option.label
          .toLowerCase()
          .includes(inputValue.toString().toLowerCase()),
    );

    // If there are no matching options but there is an input value, create a new option with the input value
    const showInputAsOption = React.useMemo(() => {
      if (!inputValue) return false;

      const hasLabelMatch = options.some(
        (option) =>
          option.label.toLowerCase() === inputValue.toString().toLowerCase(),
      );

      let isAlreadySelected = false;
      if (type === 'number') {
        const numValue = Number(inputValue);
        isAlreadySelected =
          !isNaN(numValue) && (normalizedValue as number[]).includes(numValue);
      } else if (type === 'date' || type === 'datetime') {
        const dateValue = new Date(inputValue);
        isAlreadySelected =
          !isNaN(dateValue.getTime()) &&
          (normalizedValue as Date[]).some(
            (d) => d.getTime() === dateValue.getTime(),
          );
      } else {
        isAlreadySelected = (normalizedValue as string[]).includes(inputValue);
      }
      return (
        !hasLabelMatch &&
        !isAlreadySelected &&
        inputValue.toString().trim() !== ''
      );
    }, [inputValue, options, normalizedValue, type]);

    const triggerElement = (
      <div
        className={cn(
          'flex flex-wrap items-center gap-1 w-full rounded-md border-0.5 border-border-button bg-bg-input px-3 py-1 min-h-8 cursor-text',
          'outline-none transition-colors',
          'focus-within:outline-none focus-within:ring-1 focus-within:ring-accent-primary',
          className,
        )}
        style={style}
        onClick={handleContainerClick}
      >
        {/* Render selected tags - only show tags if multi is true or if single select has a value */}
        {multi &&
          normalizedValue.map((tagValue, index) => {
            const option = options.find((opt) =>
              type === 'number'
                ? Number(opt.value) === Number(tagValue)
                : type === 'date' || type === 'datetime'
                  ? new Date(opt.value).getTime() ===
                    new Date(tagValue).getTime()
                  : String(opt.value) === String(tagValue),
            ) || {
              value: String(tagValue),
              label: String(tagValue),
            };

            return (
              <div
                key={`${tagValue}-${index}`}
                className="flex items-center bg-bg-card text-text-primary rounded px-2 py-1 text-xs mr-1 mb-1 border border-border-card truncate"
              >
                <div className="flex-1  truncate">{option.label}</div>
                <button
                  type="button"
                  className="ml-1 text-text-secondary hover:text-text-primary focus:outline-none"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleRemoveTag(tagValue);
                  }}
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            );
          })}

        {/* For single select, show the selected value as text instead of a tag */}
        {!multi && !isEmpty(normalizedValue[0]) && (
          <div className={cn('flex items-center max-w-full')}>
            <div className="flex-1  truncate">
              {options.find((opt) =>
                type === 'number'
                  ? Number(opt.value) === Number(normalizedValue[0])
                  : type === 'date' || type === 'datetime'
                    ? new Date(opt.value).getTime() ===
                      new Date(normalizedValue[0]).getTime()
                    : String(opt.value) === String(normalizedValue[0]),
              )?.label ||
                (type === 'number'
                  ? String(normalizedValue[0])
                  : type === 'date' || type === 'datetime'
                    ? new Date(normalizedValue[0] as any).toLocaleString()
                    : String(normalizedValue[0]))}
            </div>
            <button
              type="button"
              className="ml-2 flex-[0_0_24px] text-text-secondary hover:text-text-primary focus:outline-none"
              onClick={(e) => {
                e.stopPropagation();
                handleRemoveTag(normalizedValue[0]);
              }}
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        )}

        {/* Input field for adding new tags - hide if single select and value is already selected, or in multi select when not focused */}
        {(multi ? isFocused : multi || isEmpty(normalizedValue[0])) && (
          <Input
            ref={inputRef}
            type={
              type === 'date'
                ? 'date'
                : type === 'datetime'
                  ? 'datetime-local'
                  : type === 'number'
                    ? 'number'
                    : 'text'
            }
            value={
              type === 'number' && inputValue
                ? String(inputValue)
                : type === 'date' || type === 'datetime'
                  ? inputValue
                  : inputValue
            }
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            placeholder={
              (
                multi
                  ? normalizedValue.length === 0
                  : isEmpty(normalizedValue[0])
              )
                ? placeholder
                : ''
            }
            className="flex-grow min-w-[50px] border-none px-1 py-0 bg-transparent focus-visible:ring-0 focus-visible:ring-offset-0 h-auto "
            onClick={(e) => e.stopPropagation()}
            onFocus={handleInputFocus}
            onBlur={handleInputBlur}
          />
        )}
      </div>
    );

    return (
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>{triggerElement}</PopoverTrigger>
        <PopoverContent
          className="p-0 min-w-[var(--radix-popover-trigger-width)] max-w-[var(--radix-popover-trigger-width)] data-[state=open]:data-[side=top]:animate-slideDownAndFade data-[state=open]:data-[side=right]:animate-slideLeftAndFade data-[state=open]:data-[side=bottom]:animate-slideUpAndFade data-[state=open]:data-[side=left]:animate-slideRightAndFade"
          align="start"
          sideOffset={4}
          collisionPadding={4}
          onOpenAutoFocus={(e) => e.preventDefault()} // Prevent auto focus on content
        >
          <div className="max-h-60 overflow-auto">
            {filteredOptions.length > 0 &&
              filteredOptions.map((option) => (
                <div
                  key={option.value}
                  className="px-4 py-2 hover:bg-border-button cursor-pointer text-text-secondary w-full truncate"
                  onClick={() => {
                    let optionValue: any;
                    if (type === 'number') {
                      optionValue = Number(option.value);
                      if (isNaN(optionValue)) return; // Skip invalid numbers
                    } else if (type === 'date' || type === 'datetime') {
                      optionValue = new Date(option.value);
                      if (isNaN(optionValue.getTime())) return; // Skip invalid dates
                    } else {
                      optionValue = option.value;
                    }
                    handleAddTag(optionValue);
                  }}
                >
                  {option.label}
                </div>
              ))}
            {showInputAsOption && (
              <div
                key={inputValue}
                className="px-4 py-2 hover:bg-border-button cursor-pointer text-text-secondary w-full truncate"
                onClick={() =>
                  handleAddTag(
                    type === 'number'
                      ? Number(inputValue)
                      : type === 'date' || type === 'datetime'
                        ? new Date(inputValue)
                        : inputValue,
                  )
                }
              >
                {t('common.add')} &quot;{inputValue}&quot;
              </div>
            )}
            {filteredOptions.length === 0 && !showInputAsOption && (
              <div className="px-4 py-2 text-text-secondary w-full truncate">
                {t('common.noResults')}
              </div>
            )}
          </div>
        </PopoverContent>
      </Popover>
    );
  },
);

InputSelect.displayName = 'InputSelect';

export { InputSelect };
