import { cn } from '@/lib/utils';
import { Funnel } from 'lucide-react';
import React, {
  ChangeEventHandler,
  PropsWithChildren,
  ReactNode,
  useMemo,
} from 'react';
import { HomeIcon } from '../svg-icon';
import { Button, ButtonProps } from '../ui/button';
import { SearchInput } from '../ui/input';
import { CheckboxFormMultipleProps, FilterPopover } from './filter-popover';

interface IProps {
  title?: ReactNode;
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
  showFilter?: boolean;
  showSearch?: boolean;
  leftPanel?: ReactNode;
  preChildren?: ReactNode;
}

export const FilterButton = React.forwardRef<
  HTMLButtonElement,
  ButtonProps & { count?: number }
>(({ count = 0, ...props }, ref) => {
  return (
    <Button
      variant="outline"
      size={count > 0 ? 'default' : 'icon'}
      {...props}
      ref={ref}
    >
      <Funnel />

      {count > 0 && (
        <span className="rounded bg-text-badge px-1 py-0.5 text-xs leading-none text-text-primary">
          {count}
        </span>
      )}
    </Button>
  );
});

FilterButton.displayName = 'FilterButton';

export default function ListFilterBar({
  title,
  children,
  preChildren,
  searchString,
  onSearchChange,
  showFilter = true,
  showSearch = true,
  leftPanel,
  value,
  onChange,
  onOpenChange,
  filters,
  className,
  icon,
  iconClassName,
  filterGroup,
}: PropsWithChildren<IProps & Omit<CheckboxFormMultipleProps, 'setOpen'>> & {
  className?: string;
  icon?: ReactNode;
  iconClassName?: string;
  filterGroup?: Record<string, string[]>;
}) {
  const filterCount = useMemo(() => {
    return typeof value === 'object' && value !== null
      ? Object.values(value).reduce((pre, cur) => {
          if (Array.isArray(cur)) {
            return pre + cur.length;
          }
          if (typeof cur === 'object') {
            return (
              pre +
              Object.values(cur).reduce((pre, cur) => {
                return pre + cur.length;
              }, 0)
            );
          }
          return pre;
        }, 0)
      : 0;
  }, [value]);

  const hasFilter = Boolean(filters?.length && showFilter);

  return (
    <div
      className={cn(
        'flex min-w-0 flex-col gap-3 md:flex-row md:items-center md:justify-between',
        className,
      )}
    >
      <h1 className="flex min-w-0 shrink-0 items-center gap-2.5 text-2xl font-semibold">
        {typeof icon === 'string' ? (
          <HomeIcon
            name={`${icon}`}
            imgClass={cn('size-[1em]', iconClassName)}
          />
        ) : (
          icon
        )}
        {leftPanel || title}
      </h1>

      <div
        className={cn(
          'min-w-0 w-full items-center gap-2',
          preChildren
            ? 'flex flex-wrap md:flex-nowrap md:w-auto md:shrink-0 md:gap-4'
            : cn(
                'grid',
                hasFilter
                  ? 'grid-cols-[auto_minmax(0,1fr)_auto]'
                  : 'grid-cols-[minmax(0,1fr)_auto]',
                'md:flex md:w-auto md:shrink-0 md:gap-4',
              ),
        )}
        role="toolbar"
      >
        {preChildren}
        {hasFilter && (
          <FilterPopover
            value={value}
            onChange={onChange}
            filters={filters}
            filterGroup={filterGroup}
            onOpenChange={onOpenChange}
          >
            <FilterButton count={filterCount} />
          </FilterPopover>
        )}
        {showSearch && (
          <SearchInput
            value={searchString}
            onChange={onSearchChange}
            className={cn(
              'min-w-0 w-full',
              preChildren ? 'flex-1 basis-32' : '',
              'md:w-32',
            )}
            role="searchbox"
          />
        )}

        {children && (
          <div className="shrink-0 justify-self-end">{children}</div>
        )}
      </div>
    </div>
  );
}
