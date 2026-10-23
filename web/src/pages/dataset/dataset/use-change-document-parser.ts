import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useRunDocument,
  useSetDocumentPipelineParser,
  DocumentKeys,
} from '@/hooks/use-document-request';
import { useQueryClient } from '@tanstack/react-query';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const useChangeDocumentParser = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { setDocumentPipelineParser, loading } = useSetDocumentPipelineParser();
  const { runDocumentByIds } = useRunDocument();
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
          pipelineVersionId: parserConfigInfo.pipeline_version_id || '',
          documentId: record?.id,
          datasetId: record?.dataset_id,
          parserConfig: parserConfigInfo.parser_config,
          parse_type: parserConfigInfo.parse_type,
        });
        if (ret === 0) {
          // Refresh the document list so the reopened dialog reflects the newly
          // pinned version instead of the stale record (design section 8).
          queryClient.invalidateQueries({ queryKey: DocumentKeys.all() });
          hideChangeParserModal();
          if (record.chunk_count > 0) {
            Modal.confirm({
              title: t('knowledgeDetails.parseFile'),
              content: t('knowledgeDetails.parserConfigReparseTip'),
              okText: t('common.yes'),
              cancelText: t('common.no'),
              onOk: async () => {
                try {
                  await runDocumentByIds({
                    documentIds: [record.id],
                    run: 1,
                    option: { delete: true, apply_kb: false },
                  });
                } catch (error) {
                  console.warn(error);
                }
              },
            });
          }
        }
      }
    },
    [
      record?.id,
      record?.dataset_id,
      record?.chunk_count,
      runDocumentByIds,
      setDocumentPipelineParser,
      hideChangeParserModal,
      t,
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
