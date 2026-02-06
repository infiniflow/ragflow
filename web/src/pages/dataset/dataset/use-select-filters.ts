import {
  FilterCollection,
  FilterType,
} from '@/components/list-filter-bar/interface';
import { useTranslate } from '@/hooks/common-hooks';
import { useGetDocumentFilter } from '@/hooks/use-document-request';
import { useMemo } from 'react';

export const EMPTY_METADATA_FIELD = 'empty_metadata';

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
    let list = [] as FilterType[];
    if (filter.run_status) {
      list = Object.keys(filter.run_status).map((x) => ({
        id: x,
        label: t(`runningStatus${x}`),
        count: filter.run_status[x as unknown as number],
      }));
    }
    if (filter.metadata) {
      const emptyMetadata = filter.metadata?.empty_metadata;
      if (emptyMetadata) {
        list.push({
          id: EMPTY_METADATA_FIELD,
          label: t('emptyMetadata'),
          count: emptyMetadata.true,
        });
      }
    }
    return list;
  }, [filter.run_status, filter.metadata, t]);
  const metaDataList = useMemo(() => {
    if (filter.metadata) {
      const list = Object.keys(filter.metadata)
        ?.filter((m) => m !== EMPTY_METADATA_FIELD)
        ?.map((x) => {
          return {
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
          };
        });
      return list;
    }
  }, [filter.metadata]);

  const filters: FilterCollection[] = useMemo(() => {
    return [
      { field: 'type', label: 'File Type', list: fileTypes },
      { field: 'run', label: 'Status', list: fileStatus },
      {
        field: 'metadata',
        label: 'Metadata field',
        canSearch: true,
        list: metaDataList,
      },
    ] as FilterCollection[];
  }, [fileStatus, fileTypes, metaDataList]);

  const filterGroup = {
    [t('systemAttribute')]: ['type', 'run'],
    // [t('metadataField')]: ['metadata'],
  };
  return { filters, onOpenChange, filterGroup };
}
