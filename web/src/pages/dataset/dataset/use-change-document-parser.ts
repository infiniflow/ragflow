import { useSetModalState } from '@/hooks/common-hooks';
import {
  useSetDocumentParser,
  useSetDocumentPipelineParser,
} from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { isGoBackend } from '@/utils/backend-runtime';
import { useCallback, useState } from 'react';

export const useChangeDocumentParser = () => {
  const { setDocumentParser, loading } = useSetDocumentParser();
  const { setDocumentPipelineParser, loading: pipelineParserLoading } =
    useSetDocumentPipelineParser();
  const [record, setRecord] = useState<IDocumentInfo>({} as IDocumentInfo);

  const {
    visible: changeParserVisible,
    hideModal: hideChangeParserModal,
    showModal: showChangeParserModal,
  } = useSetModalState();

  const onChangeParserOk = useCallback(
    async (parserConfigInfo: IChangeParserRequestBody) => {
      if (record?.id && record?.dataset_id) {
        console.log(
          '[onChangeParserOk] parserConfigInfo.parseType:',
          parserConfigInfo.parseType,
          'parser_id:',
          parserConfigInfo.parser_id,
          'pipeline_id:',
          parserConfigInfo.pipeline_id,
        );
        // The Go document endpoint takes `parser_id` and a pipeline-shaped
        // parser_config; the Python one keeps the legacy payload shape.
        const common = {
          parserId: parserConfigInfo.parser_id,
          pipelineId: parserConfigInfo.pipeline_id || '',
          documentId: record?.id,
          datasetId: record?.dataset_id,
          parserConfig: parserConfigInfo.parser_config,
        };
        const ret = isGoBackend()
          ? await setDocumentPipelineParser({
              ...common,
              parseType: parserConfigInfo.parseType,
            })
          : await setDocumentParser(common);
        if (ret === 0) {
          hideChangeParserModal();
        }
      }
    },
    [
      record?.id,
      record?.dataset_id,
      setDocumentParser,
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
    changeParserLoading: loading || pipelineParserLoading,
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
