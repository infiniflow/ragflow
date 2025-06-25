'use client';

import { CheckIcon, ChevronDownIcon } from 'lucide-react';
import {
  Fragment,
  forwardRef,
  useCallback,
  useEffect,
  useId,
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
import { RAGFlowSelectOptionType } from '../ui/select';

const countries = [
  {
    label: 'America',
    options: [
      { value: 'United States', label: '🇺🇸' },
      { value: 'Canada', label: '🇨🇦' },
      { value: 'Mexico', label: '🇲🇽' },
    ],
  },
  {
    label: 'Africa',
    options: [
      { value: 'South Africa', label: '🇿🇦' },
      { value: 'Nigeria', label: '🇳🇬' },
      { value: 'Morocco', label: '🇲🇦' },
    ],
  },
  {
    label: 'Asia',
    options: [
      { value: 'China', label: '🇨🇳' },
      { value: 'Japan', label: '🇯🇵' },
      { value: 'India', label: '🇮🇳' },
    ],
  },
  {
    label: 'Europe',
    options: [
      { value: 'United Kingdom', label: '🇬🇧' },
      { value: 'France', label: '🇫🇷' },
      { value: 'Germany', label: '🇩🇪' },
    ],
  },
  {
    label: 'Oceania',
    options: [
      { value: 'Australia', label: '🇦🇺' },
      { value: 'New Zealand', label: '🇳🇿' },
    ],
  },
];

export type SelectWithSearchFlagOptionType = {
  label: string;
  options: RAGFlowSelectOptionType[];
};

export type SelectWithSearchFlagProps = {
  options?: SelectWithSearchFlagOptionType[];
  value?: string;
  onChange?(value: string): void;
};

export const SelectWithSearch = forwardRef<
  React.ElementRef<typeof Button>,
  SelectWithSearchFlagProps
>(({ value: val = '', onChange, options = countries }, ref) => {
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

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          variant="outline"
          role="combobox"
          aria-expanded={open}
          ref={ref}
          className="bg-background hover:bg-background border-input w-full justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px]"
        >
          {value ? (
            <span className="flex min-w-0 options-center gap-2">
              <span className="text-lg leading-none truncate">
                {
                  options
                    .map((group) =>
                      group.options.find((item) => item.value === value),
                    )
                    .filter(Boolean)[0]?.label
                }
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
            {options.map((group) => (
              <Fragment key={group.label}>
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
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
});
