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
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { get } from 'lodash';
import { ChevronDownIcon, XIcon } from 'lucide-react';
import * as React from 'react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { JsonSchemaDataType } from '../../constant';
import {
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
  parentLabel?: string;
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
  types?: JsonSchemaDataType[];
}

export function GroupedSelectWithSecondaryMenu({
  options,
  value,
  onChange,
  placeholder,
  types,
}: GroupedSelectWithSecondaryMenuProps) {
  const { t } = useTranslation();
  const [open, setOpen] = React.useState(false);

  const showSecondaryMenu = useShowSecondaryMenu();
  const findAgentStructuredOutputLabel = useFindAgentStructuredOutputLabel();

  // Find the label of the selected item
  const flattenedOptions = options.flatMap((g) => g.options);

  let selectedItem = flattenedOptions
    .flatMap((o) => [o, ...(o.children || [])])
    .find((o) => o.value === value);

  if (!selectedItem && value) {
    selectedItem = findAgentStructuredOutputLabel(value, flattenedOptions);
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
            '!bg-bg-input hover:bg-background border-input w-full  justify-between px-3 font-normal outline-offset-0 outline-none focus-visible:outline-[3px] [&_svg]:pointer-events-auto',
            !value && 'text-muted-foreground',
          )}
        >
          {value ? (
            <div className="truncate flex items-center gap-1">
              <span>{get(selectedItem, 'parentLabel')}</span>
              <span className="text-text-disabled">/</span>
              <span className="text-accent-primary">{selectedItem?.label}</span>
            </div>
          ) : (
            <span className="text-muted-foreground">
              {placeholder || t('common.selectPlaceholder')}
            </span>
          )}
          <div className="flex items-center justify-between">
            {value && (
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

      <PopoverContent className="p-0" align="start">
        <Command value={value}>
          <CommandInput placeholder="Search..." />
          <CommandList className="overflow-auto">
            {options.map((group, idx) => (
              <CommandGroup key={idx} heading={group.label}>
                {group.options.map((option) => {
                  const shouldShowSecondary = showSecondaryMenu(
                    option.value,
                    option.label,
                  );

                  if (shouldShowSecondary) {
                    return (
                      <StructuredOutputSecondaryMenu
                        key={option.value}
                        data={option}
                        click={handleSecondaryMenuClick}
                        types={types}
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
                              'cursor-pointer rounded-sm px-2 py-1.5 text-sm hover:bg-bg-card hover:text-accent-foreground',
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
                      className="flex items-center justify-between"
                    >
                      <span> {option.label}</span>
                      <span className="text-text-secondary">
                        {get(option, 'type')}
                      </span>
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
