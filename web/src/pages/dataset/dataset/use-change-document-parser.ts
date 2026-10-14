import { useSetModalState } from '@/hooks/common-hooks';
import { useSetDocumentPipelineParser } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback, useState } from 'react';

export const useChangeDocumentParser = () => {
  const { setDocumentPipelineParser, loading } = useSetDocumentPipelineParser();
  const [record, setRecord] = useState<IDocumentInfo>({} as IDocumentInfo);

  const {
    visible: changeParserVisible,
    hideModal: hideChangeParserModal,
    showModal: showChangeParserModal,
  } = useSetModalState();

  const onChangeParserOk = useCallback(
    async (parserConfigInfo: IChangeParserRequestBody) => {
      if (record?.id && record?.dataset_id) {
        // The document endpoint takes `parser_id` and a pipeline-shaped
        // parser_config.
        const ret = await setDocumentPipelineParser({
          parserId: parserConfigInfo.parser_id,
          pipelineId: parserConfigInfo.pipeline_id || '',
          documentId: record?.id,
          datasetId: record?.dataset_id,
          parserConfig: parserConfigInfo.parser_config,
          parseType: parserConfigInfo.parseType,
        });
        if (ret === 0) {
          hideChangeParserModal();
        }
      }
    },
    [
      record?.id,
      record?.dataset_id,
      setDocumentPipelineParser,
      hideChangeParserModal,
    ],
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
