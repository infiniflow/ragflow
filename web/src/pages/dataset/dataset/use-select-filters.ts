import { useFetchAllDocumentList } from '@/hooks/use-document-request';
import { groupListByType } from '@/utils/dataset-util';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export function useSelectDatasetFilters() {
  const {
    data: { docs: documents },
  } = useFetchAllDocumentList();
  const { t } = useTranslation();

  const fileTypes = useMemo(() => {
    return groupListByType(documents, 'type', 'type');
  }, [documents]);

  const fileStatus = useMemo(() => {
    return groupListByType(documents, 'run', 'run').map((x) => ({
      ...x,
      label: t(`knowledgeDetails.runningStatus${x.label}`),
    }));
  }, [documents, t]);

  const filters = useMemo(() => {
    return [
      { field: 'type', label: 'File Type', list: fileTypes },
      { field: 'run', label: 'Status', list: fileStatus },
    ];
  }, [fileStatus, fileTypes]);

  return { fileTypes, fileStatus, filters };
}
