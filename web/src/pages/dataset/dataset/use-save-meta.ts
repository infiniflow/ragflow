import { useSetModalState } from '@/hooks/common-hooks';
import { useSetDocumentMeta } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useCallback, useState } from 'react';

export const useSaveMeta = () => {
  const { setDocumentMeta, loading } = useSetDocumentMeta();
  const [record, setRecord] = useState<IDocumentInfo>({} as IDocumentInfo);

  const {
    visible: setMetaVisible,
    hideModal: hideSetMetaModal,
    showModal: showSetMetaModal,
  } = useSetModalState();

  const onSetMetaModalOk = useCallback(
    async (meta: string) => {
      const ret = await setDocumentMeta({
        documentId: record?.id,
        meta,
      });
      if (ret === 0) {
        hideSetMetaModal();
      }
    },
    [setDocumentMeta, record?.id, hideSetMetaModal],
  );

  const handleShowSetMetaModal = useCallback(
    (row: IDocumentInfo) => {
      setRecord(row);
      showSetMetaModal();
    },
    [showSetMetaModal],
  );

  return {
    setMetaLoading: loading,
    onSetMetaModalOk,
    setMetaVisible,
    hideSetMetaModal,
    showSetMetaModal: handleShowSetMetaModal,
    metaRecord: record,
  };
};

export type UseSaveMetaShowType = Pick<
  ReturnType<typeof useSaveMeta>,
  'showSetMetaModal'
>;
