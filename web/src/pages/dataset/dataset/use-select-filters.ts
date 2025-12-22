import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useTranslate } from '@/hooks/common-hooks';
import { useGetDocumentFilter } from '@/hooks/use-document-request';
import { useMemo } from 'react';

export function useSelectDatasetFilters() {
  const { t } = useTranslate('knowledgeDetails');
  const { filter, onOpenChange } = useGetDocumentFilter();

  const fileTypes = useMemo(() => {
    if (filter.suffix) {
      return Object.keys(filter.suffix).map((x) => ({
        id: x,
        label: x.toUpperCase(),
        count: filter.suffix[x],
      }));
    }
  }, [filter.suffix]);
  const fileStatus = useMemo(() => {
    if (filter.run_status) {
      return Object.keys(filter.run_status).map((x) => ({
        id: x,
        label: t(`runningStatus${x}`),
        count: filter.run_status[x as unknown as number],
      }));
    }
  }, [filter.run_status, t]);
  const metaDataList = useMemo(() => {
    if (filter.metadata) {
      return Object.keys(filter.metadata).map((x) => ({
        id: x.toString(),
        field: x.toString(),
        label: x.toString(),
        list: Object.keys(filter.metadata[x]).map((y) => ({
          id: y.toString(),
          field: y.toString(),
          label: y.toString(),
          value: [y],
          count: filter.metadata[x][y],
        })),
        count: Object.keys(filter.metadata[x]).reduce(
          (acc, cur) => acc + filter.metadata[x][cur],
          0,
        ),
      }));
    }
  }, [filter.metadata]);
  const filters: FilterCollection[] = useMemo(() => {
    return [
      { field: 'type', label: 'File Type', list: fileTypes },
      { field: 'run', label: 'Status', list: fileStatus },
      { field: 'metadata', label: 'metadata', list: metaDataList },
    ] as FilterCollection[];
  }, [fileStatus, fileTypes, metaDataList]);

  return { filters, onOpenChange };
}
