// https://github.com/sersavan/shadcn-multi-select-component
// src/components/multi-select.tsx

import { cva, type VariantProps } from 'class-variance-authority';
import {
  CheckIcon,
  ChevronDown,
  WandSparkles,
  XCircle,
  XIcon,
} from 'lucide-react';
import * as React from 'react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { isEmpty } from 'lodash';

export type MultiSelectOptionType = {
  label: React.ReactNode;
  value: string;
  disabled?: boolean;
  suffix?: React.ReactNode;
  icon?: React.ComponentType<{ className?: string }>;
};

export type MultiSelectGroupOptionType = {
  label: React.ReactNode;
  options: MultiSelectOptionType[];
};

function MultiCommandItem({
  option,
  isSelected,
  toggleOption,
}: {
  option: MultiSelectOptionType;
  isSelected: boolean;
  toggleOption(value: string): void;
}) {
  return (
    <CommandItem
      key={option.value}
      onSelect={() => {
        if (option.disabled) return false;
        toggleOption(option.value);
      }}
      className={cn('cursor-pointer', {
        'cursor-not-allowed text-text-disabled': option.disabled,
      })}
    >
      <div
        className={cn(
          'mr-2 flex h-4 w-4 items-center justify-center rounded-sm border border-primary',
          isSelected ? 'bg-primary ' : 'opacity-50 [&_svg]:invisible',

          { 'text-primary-foreground': !option.disabled },
          { 'text-text-disabled': option.disabled },
        )}
      >
        <CheckIcon className="h-4 w-4" />
      </div>
      {option.icon && (
        <option.icon
          className={cn('mr-2 h-4 w-4 ', {
            'text-text-disabled': option.disabled,
            'text-muted-foreground': !option.disabled,
          })}
        />
      )}
      <span className={cn({ 'text-text-disabled': option.disabled })}>
        {option.label}
      </span>
      {option.suffix && (
        <span className={cn({ 'text-text-disabled': option.disabled })}>
          {option.suffix}
        </span>
      )}
    </CommandItem>
  );
}

/**
 * Variants for the multi-select component to handle different styles.
 * Uses class-variance-authority (cva) to define different styles based on "variant" prop.
 */
const multiSelectVariants = cva(
  'm-1 transition ease-in-out delay-150 hover:-translate-y-1 hover:scale-110 duration-300',
  {
    variants: {
      variant: {
        default:
          'border-foreground/10 text-foreground bg-card hover:bg-card/80',
        secondary:
          'border-foreground/10 bg-secondary text-secondary-foreground hover:bg-secondary/80',
        destructive:
          'border-transparent bg-destructive text-destructive-foreground hover:bg-destructive/80',
        inverted: 'inverted',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

/**
 * Props for MultiSelect component
 */
interface MultiSelectProps
  extends
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof multiSelectVariants> {
  /**
   * An array of option objects to be displayed in the multi-select component.
   * Each option object has a label, value, and an optional icon.
   */
  options: (MultiSelectGroupOptionType | MultiSelectOptionType)[];

  /**
   * Callback function triggered when the selected values change.
   * Receives an array of the new selected values.
   */
  onValueChange: (value: string[]) => void;

  /** The default selected values when the component mounts. */
  defaultValue?: string[];

  /**
   * Placeholder text to be displayed when no values are selected.
   * Optional, defaults to "Select options".
   */
  placeholder?: string;

  /**
   * Animation duration in seconds for the visual effects (e.g., bouncing badges).
   * Optional, defaults to 0 (no animation).
   */
  animation?: number;

  /**
   * Maximum number of items to display. Extra selected items will be summarized.
   * Optional, defaults to 3.
   */
  maxCount?: number;

  /**
   * The modality of the popover. When set to true, interaction with outside elements
   * will be disabled and only popover content will be visible to screen readers.
   * Optional, defaults to false.
   */
  modalPopover?: boolean;

  /**
   * If true, renders the multi-select component as a child of another component.
   * Optional, defaults to false.
   */
  asChild?: boolean;

  /**
   * Additional class names to apply custom styles to the multi-select component.
   * Optional, can be used to add custom styles.
   */
  className?: string;

  /**
   * If true, renders the multi-select component with a select all option.
   */
  showSelectAll?: boolean;
}

export const MultiSelect = React.forwardRef<
  HTMLButtonElement,
  MultiSelectProps
>(
  (
    {
      options,
      onValueChange,
      variant,
      defaultValue = [],
      placeholder = t('common.selectPlaceholder'),
      animation = 0,
      maxCount = 3,
      modalPopover = false,
      // asChild = false,
      className,
      showSelectAll = true,
      ...props
    },
    ref,
  ) => {
    const [selectedValues, setSelectedValues] =
      React.useState<string[]>(defaultValue);
    const [isPopoverOpen, setIsPopoverOpen] = React.useState(false);
    const [isAnimating, setIsAnimating] = React.useState(false);

    React.useEffect(() => {
      if (isEmpty(selectedValues) && !isEmpty(props.value)) {
        setSelectedValues(props.value as string[]);
      }
    }, [props.value, selectedValues]);

    React.useEffect(() => {
      if (
        isEmpty(selectedValues) &&
        isEmpty(props.value) &&
        !isEmpty(defaultValue)
      ) {
        setSelectedValues(defaultValue);
      }
    }, [defaultValue, props.value, selectedValues]);

    const flatOptions = React.useMemo(() => {
      return options.flatMap((option) =>
        'options' in option ? option.options : [option],
      );
    }, [options]);
    const handleInputKeyDown = (
      event: React.KeyboardEvent<HTMLInputElement>,
    ) => {
      if (event.key === 'Enter') {
        setIsPopoverOpen(true);
      } else if (event.key === 'Backspace' && !event.currentTarget.value) {
        const newSelectedValues = [...selectedValues];
        newSelectedValues.pop();
        setSelectedValues(newSelectedValues);
        onValueChange(newSelectedValues);
      }
    };

    const toggleOption = (option: string) => {
      const newSelectedValues = selectedValues.includes(option)
        ? selectedValues.filter((value) => value !== option)
        : [...selectedValues, option];
      setSelectedValues(newSelectedValues);
      onValueChange(newSelectedValues);
    };

    const handleClear = () => {
      setSelectedValues([]);
      onValueChange([]);
    };

    const handleTogglePopover = () => {
      setIsPopoverOpen((prev) => !prev);
    };

    const clearExtraOptions = () => {
      const newSelectedValues = selectedValues.slice(0, maxCount);
      setSelectedValues(newSelectedValues);
      onValueChange(newSelectedValues);
    };

    const toggleAll = () => {
      if (selectedValues.length === flatOptions.length) {
        handleClear();
      } else {
        const allValues = flatOptions.map((option) => option.value);
        setSelectedValues(allValues);
        onValueChange(allValues);
      }
    };

    return (
      <Popover
        open={isPopoverOpen}
        onOpenChange={setIsPopoverOpen}
        modal={modalPopover}
      >
        <PopoverTrigger asChild>
          <Button
            ref={ref}
            {...props}
            onClick={handleTogglePopover}
            className={cn(
              'flex w-full p-1 rounded-md border border-border-button min-h-10 h-auto placeholder:text-text-disabled items-center justify-between bg-bg-input focus-visible:bg-bg-input hover:bg-bg-input [&_svg]:pointer-events-auto',
              className,
            )}
          >
            {selectedValues.length > 0 ? (
              <div className="flex justify-between items-center w-full group">
                <div className="flex flex-wrap items-center">
                  {selectedValues?.slice(0, maxCount)?.map((value) => {
                    const option = flatOptions.find((o) => o.value === value);
                    const IconComponent = option?.icon;
                    return (
                      <Badge
                        key={value}
                        variant="secondary"
                        className={cn(
                          isAnimating ? 'animate-bounce' : '',
                          'px-1',
                          multiSelectVariants({ variant }),
                        )}
                        style={{ animationDuration: `${animation}s` }}
                      >
                        <div className="flex justify-between items-center gap-1">
                          {IconComponent && (
                            <IconComponent className="h-4 w-4" />
                          )}
                          <div className="max-w-28 text-ellipsis overflow-hidden">
                            {option?.label}
                          </div>
                          <XCircle
                            className="h-4 w-4 cursor-pointer"
                            onClick={(event) => {
                              event.stopPropagation();
                              toggleOption(value);
                            }}
                          />
                        </div>
                      </Badge>
                    );
                  })}
                  {selectedValues.length > maxCount && (
                    <Badge
                      className={cn(
                        'bg-transparent text-foreground border-foreground/1 hover:bg-transparent',
                        isAnimating ? 'animate-bounce' : '',
                        multiSelectVariants({ variant }),
                      )}
                      style={{ animationDuration: `${animation}s` }}
                    >
                      {`+ ${selectedValues.length - maxCount} more`}
                      <XCircle
                        className="ml-2 h-4 w-4 cursor-pointer"
                        onClick={(event) => {
                          event.stopPropagation();
                          clearExtraOptions();
                        }}
                      />
                    </Badge>
                  )}
                </div>
                <div className="flex items-center justify-between ">
                  <XIcon
                    className="h-4 mx-2 cursor-pointer text-text-secondary hidden group-hover:block"
                    onClick={(event) => {
                      event.stopPropagation();
                      handleClear();
                    }}
                  />
                  <Separator
                    orientation="vertical"
                    className="min-h-6 h-full hidden group-hover:flex"
                  />
                  <ChevronDown className="h-4 mx-2 cursor-pointer text-text-secondary" />
                </div>
              </div>
            ) : (
              <div className="flex items-center justify-between w-full mx-auto">
                <span className="text-sm text-text-secondary mx-3">
                  {placeholder}
                </span>
                <ChevronDown className="h-4 cursor-pointer text-text-secondary mx-2" />
              </div>
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          className="w-auto p-0"
          align="start"
          onEscapeKeyDown={() => setIsPopoverOpen(false)}
        >
          <Command className="p-5 pb-8">
            {options && options.length > 0 && (
              <CommandInput
                placeholder={t('common.search') + '...'}
                onKeyDown={handleInputKeyDown}
              />
            )}
            <CommandList className="mt-2">
              <CommandEmpty>No results found.</CommandEmpty>
              <CommandGroup>
                {showSelectAll && options && options.length > 0 && (
                  <CommandItem
                    key="all"
                    onSelect={toggleAll}
                    className="cursor-pointer"
                  >
                    <div
                      className={cn(
                        'mr-2 flex h-4 w-4 items-center justify-center rounded-sm border border-primary',
                        selectedValues.length === flatOptions.length
                          ? 'bg-primary text-primary-foreground'
                          : 'opacity-50 [&_svg]:invisible',
                      )}
                    >
                      <CheckIcon className="h-4 w-4" />
                    </div>
                    <span>({t('common.selectAll')})</span>
                  </CommandItem>
                )}
                {!options.some((x) => 'options' in x) &&
                  (options as unknown as MultiSelectOptionType[]).map(
                    (option) => {
                      const isSelected = selectedValues.includes(option.value);
                      return (
                        <MultiCommandItem
                          option={option}
                          key={option.value}
                          isSelected={isSelected}
                          toggleOption={toggleOption}
                        ></MultiCommandItem>
                      );
                    },
                  )}
              </CommandGroup>
              {options.every((x) => 'options' in x) &&
                options.map((x, idx) => (
                  <CommandGroup heading={x.label} key={idx}>
                    {x.options.map((option) => {
                      const isSelected = selectedValues.includes(option.value);

                      return (
                        <MultiCommandItem
                          option={option}
                          key={option.value}
                          isSelected={isSelected}
                          toggleOption={toggleOption}
                        ></MultiCommandItem>
                      );
                    })}
                  </CommandGroup>
                ))}
              <CommandGroup>
                <div className=" absolute bottom-1 left-1 right-1 flex items-center justify-between mx-5 bg-bg-base border-t border-border-button">
                  <CommandSeparator />
                  {selectedValues.length > 0 && (
                    <>
                      <CommandItem
                        onSelect={handleClear}
                        className="flex-1 justify-center cursor-pointer"
                      >
                        Clear
                      </CommandItem>
                      <Separator
                        orientation="vertical"
                        className="flex min-h-6 h-full"
                      />
                    </>
                  )}
                  {options && options.length > 0 && (
                    <CommandItem
                      onSelect={() => setIsPopoverOpen(false)}
                      className="flex-1 justify-center cursor-pointer max-w-full"
                    >
                      {t('common.close')}
                    </CommandItem>
                  )}
                </div>
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
        {animation > 0 && selectedValues.length > 0 && (
          <WandSparkles
            className={cn(
              'cursor-pointer my-2 text-foreground bg-background w-3 h-3',
              isAnimating ? '' : 'text-muted-foreground',
            )}
            onClick={() => setIsAnimating(!isAnimating)}
          />
        )}
      </Popover>
    );
  },
);

MultiSelect.displayName = 'MultiSelect';
