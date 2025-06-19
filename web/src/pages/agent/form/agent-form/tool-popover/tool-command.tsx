import { Calendar, CheckIcon } from 'lucide-react';

import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import { cn } from '@/lib/utils';
import { Operator } from '@/pages/flow/constant';
import { useCallback, useEffect, useState } from 'react';

const Menus = [
  {
    label: 'Search',
    list: [
      Operator.Google,
      Operator.Bing,
      Operator.DuckDuckGo,
      Operator.Wikipedia,
      Operator.YahooFinance,
      Operator.PubMed,
      Operator.GoogleScholar,
    ],
  },
  {
    label: 'Communication',
    list: [Operator.Email],
  },
  {
    label: 'Productivity',
    list: [],
  },
  {
    label: 'Developer',
    list: [
      Operator.GitHub,
      Operator.ExeSQL,
      Operator.Invoke,
      Operator.Crawler,
      Operator.Code,
    ],
  },
];

type ToolCommandProps = {
  value?: string[];
  onChange?(values: string[]): void;
};

export function ToolCommand({ value, onChange }: ToolCommandProps) {
  const [currentValue, setCurrentValue] = useState<string[]>([]);

  const toggleOption = useCallback(
    (option: string) => {
      const newSelectedValues = currentValue.includes(option)
        ? currentValue.filter((value) => value !== option)
        : [...currentValue, option];
      setCurrentValue(newSelectedValues);
      onChange?.(newSelectedValues);
    },
    [currentValue, onChange],
  );

  useEffect(() => {
    if (Array.isArray(value)) {
      setCurrentValue(value);
    }
  }, [value]);

  return (
    <Command className="rounded-lg border shadow-md md:min-w-[450px]">
      <CommandInput placeholder="Type a command or search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        {Menus.map((x) => (
          <CommandGroup heading={x.label} key={x.label}>
            {x.list.map((y) => {
              const isSelected = currentValue.includes(y);
              return (
                <CommandItem
                  key={y}
                  className="cursor-pointer"
                  onSelect={() => toggleOption(y)}
                >
                  <div
                    className={cn(
                      'mr-2 flex h-4 w-4 items-center justify-center rounded-sm border border-primary',
                      isSelected
                        ? 'bg-primary text-primary-foreground'
                        : 'opacity-50 [&_svg]:invisible',
                    )}
                  >
                    <CheckIcon className="h-4 w-4" />
                  </div>
                  {/* {option.icon && (
                    <option.icon className="mr-2 h-4 w-4 text-muted-foreground" />
                  )} */}
                  {/* <span>{option.label}</span> */}
                  <Calendar />
                  <span>{y}</span>
                </CommandItem>
              );
            })}
          </CommandGroup>
        ))}
      </CommandList>
    </Command>
  );
}
