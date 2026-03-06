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
      {/* <span
        className={cn({
          'text-text-primary': count > 0,
          'text-text-sub-title-invert': count === 0,
        })}
      >
        Filter
      </span> */}
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
  leftPanel,
  value,
  onChange,
  onOpenChange,
  filters,
  className,
  icon,
  filterGroup,
}: PropsWithChildren<IProps & Omit<CheckboxFormMultipleProps, 'setOpen'>> & {
  className?: string;
  icon?: ReactNode;
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

  return (
    <div className={cn('flex justify-between mb-5 items-center', className)}>
      <div className="text-2xl font-semibold flex items-center gap-2.5">
        {typeof icon === 'string' ? (
          // <IconFont name={icon} className="size-6"></IconFont>
          <HomeIcon name={`${icon}`} width={'32'} />
        ) : (
          icon
        )}
        {leftPanel || title}
      </div>
      <div className="flex gap-5 items-center">
        {preChildren}
        {showFilter && (
          <FilterPopover
            value={value}
            onChange={onChange}
            filters={filters}
            filterGroup={filterGroup}
            onOpenChange={onOpenChange}
          >
            <FilterButton count={filterCount}></FilterButton>
          </FilterPopover>
        )}

        <SearchInput
          value={searchString}
          onChange={onSearchChange}
          className="w-32"
        ></SearchInput>
        {children}
      </div>
    </div>
  );
}
