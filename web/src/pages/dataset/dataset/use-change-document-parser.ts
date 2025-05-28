import { useSetModalState } from '@/hooks/common-hooks';
import { useSetDocumentParser } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback, useState } from 'react';

export const useChangeDocumentParser = () => {
  const { setDocumentParser, loading } = useSetDocumentParser();
  const [record, setRecord] = useState<IDocumentInfo>({} as IDocumentInfo);

  const {
    visible: changeParserVisible,
    hideModal: hideChangeParserModal,
    showModal: showChangeParserModal,
  } = useSetModalState();

  const onChangeParserOk = useCallback(
    async (parserConfigInfo: IChangeParserRequestBody) => {
      if (record?.id) {
        const ret = await setDocumentParser({
          parserId: parserConfigInfo.parser_id,
          documentId: record?.id,
          parserConfig: parserConfigInfo.parser_config,
        });
        if (ret === 0) {
          hideChangeParserModal();
        }
      }
    },
    [record?.id, setDocumentParser, hideChangeParserModal],
  );

  const handleShowChangeParserModal = useCallback(
    (row: IDocumentInfo) => {
      setRecord(row);
      showChangeParserModal();
    },
    [showChangeParserModal],
  );

  return {
    changeParserLoading: loading,
    onChangeParserOk,
    changeParserVisible,
    hideChangeParserModal,
    showChangeParserModal: handleShowChangeParserModal,
    changeParserRecord: record,
  };
};

export type UseChangeDocumentParserShowType = Pick<
  ReturnType<typeof useChangeDocumentParser>,
  'showChangeParserModal'
>;
