import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
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
  /** Selected values - string for single select, array for multi select */
  value?: string | string[];
  /** Callback when value changes */
  onChange?: (value: string | string[]) => void;
  /** Placeholder text */
  placeholder?: string;
  /** Additional class names */
  className?: string;
  /** Style object */
  style?: React.CSSProperties;
  /** Whether to allow multiple selections */
  multi?: boolean;
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
    },
    ref,
  ) => {
    const [inputValue, setInputValue] = React.useState('');
    const [open, setOpen] = React.useState(false);
    const [isFocused, setIsFocused] = React.useState(false);
    const inputRef = React.useRef<HTMLInputElement>(null);
    const { t } = useTranslation();

    // Normalize value to array for consistent handling
    const normalizedValue = Array.isArray(value) ? value : value ? [value] : [];

    /**
     * Removes a tag from the selected values
     * @param tagValue - The value of the tag to remove
     */
    const handleRemoveTag = (tagValue: string) => {
      const newValue = normalizedValue.filter((v) => v !== tagValue);
      // Return single value if not multi-select, otherwise return array
      onChange?.(multi ? newValue : newValue[0] || '');
    };

    /**
     * Adds a tag to the selected values
     * @param optionValue - The value of the tag to add
     */
    const handleAddTag = (optionValue: string) => {
      let newValue: string[];

      if (multi) {
        // For multi-select, add to array if not already included
        if (!normalizedValue.includes(optionValue)) {
          newValue = [...normalizedValue, optionValue];
          onChange?.(newValue);
        }
      } else {
        // For single-select, replace the value
        newValue = [optionValue];
        onChange?.(optionValue);
      }

      setInputValue('');
      setOpen(false); // Close the popover after adding a tag
    };

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const newValue = e.target.value;
      setInputValue(newValue);
      setOpen(newValue.length > 0); // Open popover when there's input

      // If input matches an option exactly, add it
      const matchedOption = options.find(
        (opt) => opt.label.toLowerCase() === newValue.toLowerCase(),
      );

      if (matchedOption && !normalizedValue.includes(matchedOption.value)) {
        handleAddTag(matchedOption.value);
      }
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
        onChange?.(multi ? newValue : newValue[0] || '');
      } else if (e.key === 'Enter' && inputValue.trim() !== '') {
        e.preventDefault();
        // Add input value as a new tag if it doesn't exist in options
        const matchedOption = options.find(
          (opt) => opt.label.toLowerCase() === inputValue.toLowerCase(),
        );

        if (matchedOption) {
          handleAddTag(matchedOption.value);
        } else {
          // If not in options, create a new tag with the input value
          if (
            !normalizedValue.includes(inputValue) &&
            inputValue.trim() !== ''
          ) {
            handleAddTag(inputValue);
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
      ? options.filter((option) => !normalizedValue.includes(option.value))
      : options;

    const filteredOptions = availableOptions.filter(
      (option) =>
        !inputValue ||
        option.label.toLowerCase().includes(inputValue.toLowerCase()),
    );

    // If there are no matching options but there is an input value, create a new option with the input value
    const hasMatchingOptions = filteredOptions.length > 0;
    const showInputAsOption =
      inputValue &&
      !hasMatchingOptions &&
      !normalizedValue.includes(inputValue);

    const triggerElement = (
      <div
        className={cn(
          'flex flex-wrap items-center gap-1 w-full rounded-md border-0.5 border-border-button bg-bg-input px-3 py-2 min-h-[40px] cursor-text',
          'outline-none transition-colors',
          'focus-within:outline-none focus-within:ring-1 focus-within:ring-accent-primary',
          className,
        )}
        style={style}
        onClick={handleContainerClick}
      >
        {/* Render selected tags - only show tags if multi is true or if single select has a value */}
        {multi &&
          normalizedValue.map((tagValue) => {
            const option = options.find((opt) => opt.value === tagValue) || {
              value: tagValue,
              label: tagValue,
            };
            return (
              <div
                key={tagValue}
                className="flex items-center bg-bg-card text-text-primary rounded px-2 py-1 text-xs mr-1 mb-1 border border-border-card"
              >
                {option.label}
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
        {!multi && normalizedValue[0] && (
          <div className="flex items-center mr-2 max-w-full">
            <div className="flex-1  truncate">
              {options.find((opt) => opt.value === normalizedValue[0])?.label ||
                normalizedValue[0]}
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
        {(multi ? isFocused : multi || !normalizedValue[0]) && (
          <Input
            ref={inputRef}
            type="text"
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            placeholder={
              (multi ? normalizedValue.length === 0 : !normalizedValue[0])
                ? placeholder
                : ''
            }
            className="flex-grow min-w-[50px] border-none px-1 py-0 bg-transparent focus-visible:ring-0 focus-visible:ring-offset-0 h-auto !w-fit"
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
                  onClick={() => handleAddTag(option.value)}
                >
                  {option.label}
                </div>
              ))}
            {showInputAsOption && (
              <div
                key={inputValue}
                className="px-4 py-2 hover:bg-border-button cursor-pointer text-text-secondary w-full truncate"
                onClick={() => handleAddTag(inputValue)}
              >
                {t('common.add')} &quot;{inputValue}&#34;
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
