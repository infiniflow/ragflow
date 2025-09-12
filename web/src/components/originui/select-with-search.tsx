'use client';

import { CheckIcon, ChevronDownIcon, XIcon } from 'lucide-react';
import {
  Fragment,
  MouseEventHandler,
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
import { t } from 'i18next';
import { RAGFlowSelectOptionType } from '../ui/select';
import { Separator } from '../ui/separator';

export type SelectWithSearchFlagOptionType = {
  label: ReactNode;
  value?: string;
  disabled?: boolean;
  options?: RAGFlowSelectOptionType[];
};

export type SelectWithSearchFlagProps = {
  options?: SelectWithSearchFlagOptionType[];
  value?: string;
  onChange?(value: string): void;
  triggerClassName?: string;
  allowClear?: boolean;
};

export const SelectWithSearch = forwardRef<
  React.ElementRef<typeof Button>,
  SelectWithSearchFlagProps
>(
  (
    {
      value: val = '',
      onChange,
      options = [],
      triggerClassName,
      allowClear = false,
    },
    ref,
  ) => {
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

    const handleClear: MouseEventHandler<SVGElement> = useCallback(
      (e) => {
        e.stopPropagation();
        setValue('');
        onChange?.('');
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
              'bg-background hover:bg-background border-input w-full  justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px] [&_svg]:pointer-events-auto',
              triggerClassName,
            )}
          >
            {value ? (
              <span className="flex min-w-0 options-center gap-2">
                <span className="leading-none truncate">{selectLabel}</span>
              </span>
            ) : (
              <span className="text-muted-foreground">
                {t('common.selectPlaceholder')}
              </span>
            )}
            <div className="flex items-center justify-between">
              {value && allowClear && (
                <>
                  <XIcon
                    className="h-4 mx-2 cursor-pointer text-muted-foreground"
                    onClick={handleClear}
                  />
                  <Separator
                    orientation="vertical"
                    className="flex min-h-6 h-full"
                  />
                </>
              )}
              <ChevronDownIcon
                size={16}
                className="text-muted-foreground/80 shrink-0 ml-2"
                aria-hidden="true"
              />
            </div>
          </Button>
        </PopoverTrigger>
        <PopoverContent
          className="border-input w-full min-w-[var(--radix-popper-anchor-width)] p-0"
          align="start"
        >
          <Command>
            <CommandInput placeholder={t('common.search') + '...'} />
            <CommandList>
              <CommandEmpty>{t('common.noDataFound')}</CommandEmpty>
              {options.map((group, idx) => {
                if (group.options) {
                  return (
                    <Fragment key={idx}>
                      <CommandGroup heading={group.label}>
                        {group.options.map((option) => (
                          <CommandItem
                            key={option.value}
                            value={option.value}
                            disabled={option.disabled}
                            onSelect={handleSelect}
                          >
                            <span className="leading-none">{option.label}</span>

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
                      disabled={group.disabled}
                      onSelect={handleSelect}
                    >
                      <span className="leading-none">{group.label}</span>

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
  },
);

SelectWithSearch.displayName = 'SelectWithSearch';
