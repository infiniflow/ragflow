import { ChevronDown } from 'lucide-react';
import React, {
  ChangeEventHandler,
  FunctionComponent,
  PropsWithChildren,
  ReactNode,
} from 'react';
import { Button, ButtonProps } from './ui/button';
import { SearchInput } from './ui/input';

interface IProps {
  title?: string;
  FilterPopover?: FunctionComponent<any>;
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
  count?: number;
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
  FilterPopover,
  searchString,
  onSearchChange,
  count,
  showFilter = true,
  leftPanel,
}: PropsWithChildren<IProps>) {
  return (
    <div className="flex justify-between mb-6 items-center">
      <span className="text-3xl font-bold ">{leftPanel || title}</span>
      <div className="flex gap-4 items-center">
        {showFilter &&
          (FilterPopover ? (
            <FilterPopover>
              <FilterButton count={count}></FilterButton>
            </FilterPopover>
          ) : (
            <FilterButton></FilterButton>
          ))}

        <SearchInput
          value={searchString}
          onChange={onSearchChange}
        ></SearchInput>
        {children}
      </div>
    </div>
  );
}
