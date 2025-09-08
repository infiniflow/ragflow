import { cn } from '@/lib/utils';
import { Funnel } from 'lucide-react';
import React, {
  ChangeEventHandler,
  PropsWithChildren,
  ReactNode,
  useMemo,
} from 'react';
import { IconFont } from '../icon-font';
import { Button, ButtonProps } from '../ui/button';
import { SearchInput } from '../ui/input';
import { CheckboxFormMultipleProps, FilterPopover } from './filter-popover';

interface IProps {
  title?: ReactNode;
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
  showFilter?: boolean;
  leftPanel?: ReactNode;
}

export const FilterButton = React.forwardRef<
  HTMLButtonElement,
  ButtonProps & { count?: number }
>(({ count = 0, ...props }, ref) => {
  return (
    <Button variant="secondary" {...props} ref={ref}>
      {/* <span
        className={cn({
          'text-text-primary': count > 0,
          'text-text-sub-title-invert': count === 0,
        })}
      >
        Filter
      </span> */}
      {count > 0 && (
        <span className="rounded-full bg-text-badge px-1 text-xs ">
          {count}
        </span>
      )}
      <Funnel />
    </Button>
  );
});

export default function ListFilterBar({
  title,
  children,
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
}: PropsWithChildren<IProps & Omit<CheckboxFormMultipleProps, 'setOpen'>> & {
  className?: string;
  icon?: ReactNode;
}) {
  const filterCount = useMemo(() => {
    return typeof value === 'object' && value !== null
      ? Object.values(value).reduce((pre, cur) => {
          return pre + cur.length;
        }, 0)
      : 0;
  }, [value]);

  return (
    <div className={cn('flex justify-between mb-5 items-center', className)}>
      <div className="text-2xl font-semibold flex items-center gap-2.5">
        {typeof icon === 'string' ? (
          <IconFont name={icon} className="size-6"></IconFont>
        ) : (
          icon
        )}
        {leftPanel || title}
      </div>
      <div className="flex gap-5 items-center">
        {showFilter && (
          <FilterPopover
            value={value}
            onChange={onChange}
            filters={filters}
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
