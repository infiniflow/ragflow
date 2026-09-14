import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import { useRunDocument } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { buildMissingModelModalContent } from './parser-model-gap-content';
import { useParserModelValidation } from './use-parser-model-validation';

export const useHandleRunDocumentByIds = (id: string) => {
  const { t } = useTranslation();
  const { runDocumentByIds, loading } = useRunDocument();
  const [currentId, setCurrentId] = useState<string>('');
  const isLoading = loading && currentId !== '' && currentId === id;
  const { visible, showModal, hideModal } = useSetModalState();
  const { findFilesMissingModels, goToDatasetConfiguration } =
    useParserModelValidation();

  const handleRunDocumentByIds = async (
    record: IDocumentInfo,
    isRunning: boolean,
    option?: { delete: boolean; apply_kb: boolean },
  ) => {
    if (isLoading) {
      return;
    }
    // Starting a parse requires the models of the file type to be configured;
    // cancelling is always allowed.
    if (!isRunning) {
      const gaps = findFilesMissingModels([record.name]);
      if (gaps.length > 0) {
        hideModal();
        Modal.error({
          title: t('knowledgeDetails.parseBlockedTitle'),
          content: buildMissingModelModalContent(
            t,
            gaps,
            'knowledgeDetails.parseBlockedHint',
          ),
          okText: t('knowledgeDetails.goToConfiguration'),
          cancelText: t('common.cancel'),
          closable: false,
          onOk: goToDatasetConfiguration,
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
