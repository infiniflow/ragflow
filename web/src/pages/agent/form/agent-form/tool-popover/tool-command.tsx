import { Calendar, CheckIcon } from 'lucide-react';

import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import { useListMcpServer } from '@/hooks/use-mcp-request';
import { cn } from '@/lib/utils';
import { Operator } from '@/pages/agent/constant';
import { PropsWithChildren, useCallback, useEffect, useState } from 'react';

const Menus = [
  {
    label: 'Search',
    list: [
      Operator.TavilySearch,
      Operator.TavilyExtract,
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
      Operator.Code,
      Operator.Retrieval,
    ],
  },
];

type ToolCommandProps = {
  value?: string[];
  onChange?(values: string[]): void;
};

type ToolCommandItemProps = {
  toggleOption(id: string): void;
  id: string;
  isSelected: boolean;
} & ToolCommandProps;

function ToolCommandItem({
  toggleOption,
  id,
  isSelected,
  children,
}: ToolCommandItemProps & PropsWithChildren) {
  return (
    <CommandItem className="cursor-pointer" onSelect={() => toggleOption(id)}>
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
      {children}
    </CommandItem>
  );
}

function useHandleSelectChange({ onChange, value }: ToolCommandProps) {
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

  return {
    toggleOption,
    currentValue,
  };
}

export function ToolCommand({ value, onChange }: ToolCommandProps) {
  const { toggleOption, currentValue } = useHandleSelectChange({
    onChange,
    value,
  });

  return (
    <Command>
      <CommandInput placeholder="Type a command or search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        {Menus.map((x) => (
          <CommandGroup heading={x.label} key={x.label}>
            {x.list.map((y) => {
              const isSelected = currentValue.includes(y);
              return (
                <ToolCommandItem
                  key={y}
                  id={y}
                  toggleOption={toggleOption}
                  isSelected={isSelected}
                >
                  <>
                    <Calendar />
                    <span>{y}</span>
                  </>
                </ToolCommandItem>
              );
            })}
          </CommandGroup>
        ))}
      </CommandList>
    </Command>
  );
}

export function MCPCommand({ onChange, value }: ToolCommandProps) {
  const { data } = useListMcpServer();
  const { toggleOption, currentValue } = useHandleSelectChange({
    onChange,
    value,
  });

  return (
    <Command>
      <CommandInput placeholder="Type a command or search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        {data.mcp_servers.map((item) => {
          const isSelected = currentValue.includes(item.id);

          return (
            <ToolCommandItem
              key={item.id}
              id={item.id}
              isSelected={isSelected}
              toggleOption={toggleOption}
            >
              {item.name}
            </ToolCommandItem>
          );
        })}
      </CommandList>
    </Command>
  );
}
