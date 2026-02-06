import { FilterButton } from '@/components/list-filter-bar';
import {
  CheckboxFormMultipleProps,
  FilterPopover,
} from '@/components/list-filter-bar/filter-popover';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { ChangeEventHandler, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { LogTabs } from './dataset-common';

interface IProps {
  searchString?: string;
  onSearchChange?: ChangeEventHandler<HTMLInputElement>;
  active?: (typeof LogTabs)[keyof typeof LogTabs];
  setActive?: (active: (typeof LogTabs)[keyof typeof LogTabs]) => void;
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
    active = LogTabs.FILE_LOGS,
    setActive,
    ...rest
  } = props;
  const { t } = useTranslation();
  const filterCount = useMemo(() => {
    return typeof value === 'object' && value !== null
      ? Object.values(value).reduce((pre, cur) => {
          return pre + cur.length;
        }, 0)
      : 0;
  }, [value]);
  return (
    <div className="flex items-center justify-between mb-4">
      <div className="flex space-x-2 bg-bg-card p-1 rounded-md">
        <Button
          className={cn(
            'px-4 py-2 rounded-md hover:text-text-primary hover:bg-bg-base',
            {
              'bg-bg-base text-text-primary': active === LogTabs.FILE_LOGS,
              'bg-transparent text-text-secondary ':
                active !== LogTabs.FILE_LOGS,
            },
          )}
          onClick={() => setActive?.(LogTabs.FILE_LOGS)}
        >
          {t('knowledgeDetails.fileLogs')}
        </Button>
        <Button
          className={cn(
            'px-4 py-2 rounded-md hover:text-text-primary hover:bg-bg-base',
            {
              'bg-bg-base text-text-primary': active === LogTabs.DATASET_LOGS,
              'bg-transparent text-text-secondary ':
                active !== LogTabs.DATASET_LOGS,
            },
          )}
          onClick={() => setActive?.(LogTabs.DATASET_LOGS)}
        >
          {t('knowledgeDetails.datasetLogs')}
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
