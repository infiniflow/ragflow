import { Button } from '@/components/ui/button';
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { ChevronDown, X } from 'lucide-react';
import * as React from 'react';
import { useCallback } from 'react';
import {
  useFilterStructuredOutputByValue,
  useFindAgentStructuredOutputLabel,
  useShowSecondaryMenu,
} from '../../hooks/use-build-structured-output';
import { StructuredOutputSecondaryMenu } from './structured-output-secondary-menu';

type Item = {
  label: string;
  value: string;
};

type Option = {
  label: string;
  value: string;
  children?: Item[];
};

type Group = {
  label: string | React.ReactNode;
  options: Option[];
};

interface GroupedSelectWithSecondaryMenuProps {
  options: Group[];
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
}

export function GroupedSelectWithSecondaryMenu({
  options,
  value,
  onChange,
  placeholder = 'Select an option...',
}: GroupedSelectWithSecondaryMenuProps) {
  const [open, setOpen] = React.useState(false);

  const showSecondaryMenu = useShowSecondaryMenu();
  const filterStructuredOutput = useFilterStructuredOutputByValue();
  const findAgentStructuredOutputLabel = useFindAgentStructuredOutputLabel();

  // Find the label of the selected item
  const flattenedOptions = options.flatMap((g) => g.options);
  let selectedLabel =
    flattenedOptions
      .flatMap((o) => [o, ...(o.children || [])])
      .find((o) => o.value === value)?.label || '';

  if (!selectedLabel && value) {
    selectedLabel =
      findAgentStructuredOutputLabel(value, flattenedOptions)?.label ?? '';
  }

  // Handle clear click
  const handleClear = (e: React.MouseEvent) => {
    e.stopPropagation();
    onChange?.('');
    setOpen(false);
  };

  const handleSecondaryMenuClick = useCallback(
    (record: Item) => {
      onChange?.(record.value);
      setOpen(false);
    },
    [onChange],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className={cn(
            'w-full justify-between text-sm font-normal',
            !value && 'text-muted-foreground',
          )}
        >
          <span className="truncate">{selectedLabel || placeholder}</span>
          <div className="flex items-center gap-1">
            {value && (
              <X
                className="h-4 w-4 text-muted-foreground hover:text-foreground cursor-pointer"
                onClick={handleClear}
              />
            )}
            <ChevronDown className="h-4 w-4 opacity-50" />
          </div>
        </Button>
      </PopoverTrigger>

      <PopoverContent className="p-0" align="start">
        <Command>
          <CommandInput placeholder="Search..." />
          <CommandList className="overflow-visible">
            {options.map((group, idx) => (
              <CommandGroup key={idx} heading={group.label}>
                {group.options.map((option) => {
                  const shouldShowSecondary = showSecondaryMenu(
                    option.value,
                    option.label,
                  );

                  if (shouldShowSecondary) {
                    const filteredStructuredOutput = filterStructuredOutput(
                      option.value,
                    );
                    return (
                      <StructuredOutputSecondaryMenu
                        key={option.value}
                        data={option}
                        click={handleSecondaryMenuClick}
                        filteredStructuredOutput={filteredStructuredOutput}
                      ></StructuredOutputSecondaryMenu>
                    );
                  }

                  return option.children ? (
                    <HoverCard
                      key={option.value}
                      openDelay={100}
                      closeDelay={150}
                    >
                      <HoverCardTrigger asChild>
                        <CommandItem
                          onSelect={() => {}}
                          className="flex items-center justify-between cursor-default"
                        >
                          {option.label}
                          <span className="ml-auto text-muted-foreground">
                            ›
                          </span>
                        </CommandItem>
                      </HoverCardTrigger>
                      <HoverCardContent
                        side="right"
                        align="start"
                        className="w-[180px] p-1"
                      >
                        {option.children.map((child) => (
                          <div
                            key={child.value}
                            className={cn(
                              'cursor-pointer rounded-sm px-2 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground',
                              value === child.value &&
                                'bg-accent text-accent-foreground',
                            )}
                            onClick={() => {
                              onChange?.(child.value);
                              setOpen(false);
                            }}
                          >
                            {child.label}
                          </div>
                        ))}
                      </HoverCardContent>
                    </HoverCard>
                  ) : (
                    <CommandItem
                      key={option.value}
                      onSelect={() => {
                        onChange?.(option.value);
                        setOpen(false);
                      }}
                      className={cn(
                        value === option.value &&
                          'bg-accent text-accent-foreground',
                      )}
                    >
                      {option.label}
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
