import { ChevronDown } from 'lucide-react';
import React, {
  ChangeEventHandler,
  FunctionComponent,
  PropsWithChildren,
} from 'react';
import { Button, ButtonProps } from './ui/button';
import { SearchInput } from './ui/input';

interface IProps {
  title: string;
  showDialog?: () => void;
  FilterPopover?: FunctionComponent<any>;
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
  count?: number;
  showFilter?: boolean;
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
  showDialog,
  FilterPopover,
  searchString,
  onSearchChange,
  count,
  showFilter = true,
}: PropsWithChildren<IProps>) {
  return (
    <div className="flex justify-between mb-6">
      <span className="text-3xl font-bold ">{title}</span>
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
        <Button variant={'tertiary'} size={'sm'} onClick={showDialog}>
          {children}
        </Button>
      </div>
    </div>
  );
}
