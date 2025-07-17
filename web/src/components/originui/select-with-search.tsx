'use client';

import { CheckIcon, ChevronDownIcon } from 'lucide-react';
import {
  Fragment,
  ReactNode,
  forwardRef,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
} from 'react';

import { Button } from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { RAGFlowSelectOptionType } from '../ui/select';

export type SelectWithSearchFlagOptionType = {
  label: ReactNode;
  value?: string;
  options?: RAGFlowSelectOptionType[];
};

export type SelectWithSearchFlagProps = {
  options?: SelectWithSearchFlagOptionType[];
  value?: string;
  onChange?(value: string): void;
  triggerClassName?: string;
};

export const SelectWithSearch = forwardRef<
  React.ElementRef<typeof Button>,
  SelectWithSearchFlagProps
>(({ value: val = '', onChange, options = [], triggerClassName }, ref) => {
  const id = useId();
  const [open, setOpen] = useState<boolean>(false);
  const [value, setValue] = useState<string>('');

  const handleSelect = useCallback(
    (val: string) => {
      setValue(val);
      setOpen(false);
      onChange?.(val);
    },
    [onChange],
  );

  useEffect(() => {
    setValue(val);
  }, [val]);
  const selectLabel = useMemo(() => {
    const optionTemp = options[0];
    if (optionTemp?.options) {
      return options
        .map((group) => group?.options?.find((item) => item.value === value))
        .filter(Boolean)[0]?.label;
    } else {
      return options.find((opt) => opt.value === value)?.label || '';
    }
  }, [options, value]);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          variant="outline"
          role="combobox"
          aria-expanded={open}
          ref={ref}
          className={cn(
            'bg-background hover:bg-background border-input w-full justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px]',
            triggerClassName,
          )}
        >
          {value ? (
            <span className="flex min-w-0 options-center gap-2">
              <span className="text-lg leading-none truncate">
                {selectLabel}
              </span>
            </span>
          ) : (
            <span className="text-muted-foreground">Select value</span>
          )}
          <ChevronDownIcon
            size={16}
            className="text-muted-foreground/80 shrink-0"
            aria-hidden="true"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="border-input w-full min-w-[var(--radix-popper-anchor-width)] p-0"
        align="start"
      >
        <Command>
          <CommandInput placeholder="Search ..." />
          <CommandList>
            <CommandEmpty>No data found.</CommandEmpty>
            {options.map((group, idx) => {
              if (group.options) {
                return (
                  <Fragment key={idx}>
                    <CommandGroup heading={group.label}>
                      {group.options.map((option) => (
                        <CommandItem
                          key={option.value}
                          value={option.value}
                          onSelect={handleSelect}
                        >
                          <span className="text-lg leading-none">
                            {option.label}
                          </span>

                          {value === option.value && (
                            <CheckIcon size={16} className="ml-auto" />
                          )}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </Fragment>
                );
              } else {
                return (
                  <CommandItem
                    key={group.value}
                    value={group.value}
                    onSelect={handleSelect}
                  >
                    <span className="text-lg leading-none">{group.label}</span>

                    {value === group.value && (
                      <CheckIcon size={16} className="ml-auto" />
                    )}
                  </CommandItem>
                );
              }
            })}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
});

SelectWithSearch.displayName = 'SelectWithSearch';
