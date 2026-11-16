import { FilterButton } from '@/components/list-filter-bar';
import {
  CheckboxFormMultipleProps,
  FilterPopover,
} from '@/components/list-filter-bar/filter-popover';
import { SearchInput } from '@/components/ui/input';
import { Segmented } from '@/components/ui/segmented';
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
  } = props;
  const { t } = useTranslation();
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
                return pre + (cur?.length || 0);
              }, 0)
            );
          }
          return pre;
        }, 0)
      : 0;
  }, [value]);
  return (
    <div className="flex items-center justify-between mb-4 shrink-0">
      <div>
        <Segmented
          value={active}
          options={[
            { value: LogTabs.FILE_LOGS, label: t('knowledgeDetails.fileLogs') },
            {
              value: LogTabs.DATASET_LOGS,
              label: t('knowledgeDetails.datasetLogs'),
            },
            {
              value: LogTabs.DATASOURCE_LOGS,
              label: t('knowledgeDetails.datasourceLogs'),
            },
          ]}
          onChange={(value) =>
            setActive?.(value as (typeof LogTabs)[keyof typeof LogTabs])
          }
        />
      </div>

      {active !== LogTabs.DATASOURCE_LOGS && (
        <div className="flex items-center gap-4">
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
      )}
    </div>
  );
};

export { DatasetFilter };
