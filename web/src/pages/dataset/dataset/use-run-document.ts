import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import { useRunDocument } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { buildParserGapModalContent } from './parser-gap-content';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useParserGapValidation } from './use-parser-gap-validation';

export const useHandleRunDocumentByIds = (
  id: string,
  showChangeParserModal: UseChangeDocumentParserShowType['showChangeParserModal'],
) => {
  const { t } = useTranslation();
  const { runDocumentByIds, loading } = useRunDocument();
  const [currentId, setCurrentId] = useState<string>('');
  const isLoading = loading && currentId !== '' && currentId === id;
  const { visible, showModal, hideModal } = useSetModalState();
  const { findDocumentParseGaps } = useParserGapValidation();

  const handleRunDocumentByIds = async (
    record: IDocumentInfo,
    isRunning: boolean,
    option?: { delete: boolean; apply_kb: boolean },
  ) => {
    if (isLoading) {
      return;
    }
    // Starting a parse requires the file type to be supported by the
    // Parser operator the document actually runs with; cancelling is
    // always allowed.
    if (!isRunning) {
      const gaps = findDocumentParseGaps([record]);
      if (gaps.length > 0) {
        hideModal();
        Modal.error({
          title: t('knowledgeDetails.parseBlockedTitle'),
          content: buildParserGapModalContent(
            t,
            gaps,
            'knowledgeDetails.reselectParserToParseHint',
          ),
          okText: t('knowledgeDetails.reselectParser'),
          cancelText: t('common.cancel'),
          closable: false,
          onOk: () => {
            showChangeParserModal(record);
          },
        });
        return;
      }
    }
    setCurrentId(record.id);
    try {
      await runDocumentByIds({
        documentIds: [record.id],
        run: isRunning ? 2 : 1,
        option,
      });
      setCurrentId('');
    } catch (error) {
      console.warn(error);
      setCurrentId('');
    }
    hideModal();
  };

  return {
    handleRunDocumentByIds,
    loading: isLoading,
    visible,
    showModal,
    hideModal,
  };
};
