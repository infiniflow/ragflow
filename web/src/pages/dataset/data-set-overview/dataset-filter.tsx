import { FilterButton } from '@/components/list-filter-bar';
import {
  CheckboxFormMultipleProps,
  FilterPopover,
} from '@/components/list-filter-bar/filter-popover';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { ChangeEventHandler, useMemo } from 'react';
interface IProps {
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
}
const DatasetFilter = (
  props: IProps & Omit<CheckboxFormMultipleProps, 'setOpen'>,
) => {
  const {
    searchString,
    onSearchChange,
    value,
    onChange,
    filters,
    onOpenChange,
    ...rest
  } = props;
  const filterCount = useMemo(() => {
    return typeof value === 'object' && value !== null
      ? Object.values(value).reduce((pre, cur) => {
          return pre + cur.length;
        }, 0)
      : 0;
  }, [value]);
  return (
    <div className="flex items-center justify-between mb-4">
      <div className="flex space-x-2">
        <Button className="px-4 py-2 bg-blue-600 text-white rounded-md">
          File Logs
        </Button>
        <Button className="px-4 py-2 bg-gray-700 text-gray-300 rounded-md">
          Dataset Logs
        </Button>
      </div>
      <div className="flex items-center space-x-2">
        <FilterPopover
          value={value}
          onChange={onChange}
          filters={filters}
          onOpenChange={onOpenChange}
        >
          <FilterButton count={filterCount}></FilterButton>
        </FilterPopover>

        <SearchInput
          value={searchString}
          onChange={onSearchChange}
          className="w-32"
        ></SearchInput>
      </div>
    </div>
  );
};

export { DatasetFilter };
