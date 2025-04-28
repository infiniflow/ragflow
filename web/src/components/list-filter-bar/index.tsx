import { ChevronDown } from 'lucide-react';
import React, {
  ChangeEventHandler,
  PropsWithChildren,
  ReactNode,
  useMemo,
} from 'react';
import { Button, ButtonProps } from '../ui/button';
import { SearchInput } from '../ui/input';
import { CheckboxFormMultipleProps, FilterPopover } from './filter-popover';

interface IProps {
  title?: string;
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
  showFilter?: boolean;
  leftPanel?: ReactNode;
}

const FilterButton = React.forwardRef<
  HTMLButtonElement,
  ButtonProps & { count?: number }
>(({ count = 0, ...props }, ref) => {
  return (
    <Button variant="outline" size={'sm'} {...props} ref={ref}>
      Filter <span>{count}</span> <ChevronDown />
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
  filters,
}: PropsWithChildren<IProps & Omit<CheckboxFormMultipleProps, 'setOpen'>>) {
  const filterCount = useMemo(() => {
    return typeof value === 'object' && value !== null
      ? Object.values(value).reduce((pre, cur) => {
          return pre + cur.length;
        }, 0)
      : 0;
  }, [value]);

  return (
    <div className="flex justify-between mb-6 items-center">
      <span className="text-3xl font-bold ">{leftPanel || title}</span>
      <div className="flex gap-4 items-center">
        {showFilter && (
          <FilterPopover value={value} onChange={onChange} filters={filters}>
            <FilterButton count={filterCount}></FilterButton>
          </FilterPopover>
        )}

        <SearchInput
          value={searchString}
          onChange={onSearchChange}
        ></SearchInput>
        {children}
      </div>
    </div>
  );
}
