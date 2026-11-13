import { Modal } from '@/components/ui/modal/modal';
import { FileType } from '@/constants/file';
import { useSetModalState } from '@/hooks/common-hooks';
import { useRunDocument } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { buildParserGapModalContent } from './parser-gap-content';
import { ParserGapReason } from './constant';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { useParserGapValidation } from './use-parser-gap-validation';
import { pickByGapKind } from './utils';

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
        // Video has no parser that can process it, so offering "reselect parse
        // method" would send the user into a dialog that cannot fix anything.
        // Acknowledge instead; the change-parser route stays for the other
        // gap kinds. The reason check keeps this narrow: a future gap kind on
        // a parseable file must keep the actionable dialog.
        const videoOnly =
          gaps.length > 0 &&
          gaps.every(
            (gap) =>
              gap.fileType === FileType.Video &&
              gap.reason === ParserGapReason.UnsupportedType,
          );
        Modal.error({
          title: t('knowledgeDetails.parseBlockedTitle'),
          content: buildParserGapModalContent(t, gaps, {
            missingModel: 'knowledgeDetails.addModelToParseHint',
            unsupportedType: 'knowledgeDetails.reselectParserToParseHint',
          }),
          okText: t(
            videoOnly
              ? 'common.ok'
              : pickByGapKind(gaps, {
                  missingModel: 'knowledgeDetails.goAddModel',
                  unsupportedType: 'knowledgeDetails.reselectParser',
                }),
          ),
          cancelText: t('common.cancel'),
          showCancel: !videoOnly,
          closable: false,
          onOk: videoOnly ? undefined : () => showChangeParserModal(record),
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
