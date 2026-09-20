import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  UseRowSelectionType,
  useSelectedIds,
} from '@/hooks/logic-hooks/use-row-selection';
import {
  useRemoveDocument,
  useRunDocument,
  useSetDocumentStatus,
} from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useIsGoBackend } from '@/utils/backend-variant';
import {
  LucideCircleX,
  LucideCylinder,
  LucidePlayCircle,
  LucideToggleLeft,
  LucideToggleRight,
  LucideTrash2,
} from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { toast } from 'sonner';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import { DocumentType } from './constant';
import { buildParserGapModalContent } from './parser-gap-content';
import { useParserGapValidation } from './use-parser-gap-validation';
import { isDocumentProcessing } from './utils';

export function useBulkOperateDataset({
  rowSelection,
  setRowSelection,
  documents,
}: Pick<UseRowSelectionType, 'rowSelection' | 'setRowSelection'> & {
  documents: IDocumentInfo[];
}) {
  const { t } = useTranslation();
  const { selectedIds: selectedRowKeys } = useSelectedIds(
    rowSelection,
    documents,
  );
  const { id } = useParams();

  const { runDocumentByIds } = useRunDocument();
  const { setDocumentStatus } = useSetDocumentStatus();
  const { removeDocument } = useRemoveDocument();
  const { visible, showModal, hideModal } = useSetModalState();
  const { findDocumentParseGaps } = useParserGapValidation();
  const { knowledgeBase } = useKnowledgeBaseContext();
  const isGo = useIsGoBackend();

  const chunkNum = useMemo(() => {
    if (!documents.length) {
      return 0;
    }
    return documents
      .filter((item) => selectedRowKeys.includes(item.id) && item.id)
      ?.reduce((acc, cur) => {
        return acc + cur.chunk_count;
      }, 0);
  }, [documents, selectedRowKeys]);

  const runDocument = useCallback(
    async (run: number, option?: { delete: boolean; apply_kb: boolean }) => {
      const nonVirtualKeys = selectedRowKeys.filter(
        (x) =>
          !documents.some((y) => x === y.id && y.type === DocumentType.Virtual),
      );

      if (nonVirtualKeys.length === 0) {
        toast.error(t('Please select a non-empty file list'));
        return;
      }

      // Starting a parse requires each file type to be supported by the
      // Parser operator its document actually runs with; cancelling is
      // always allowed.
      if (run === 1) {
        const selectedDocuments = documents.filter((x) =>
          nonVirtualKeys.includes(x.id),
        );
        const gaps = findDocumentParseGaps(selectedDocuments);
        if (gaps.length > 0) {
          hideModal();
          const failingNames = new Set(gaps.map((gap) => gap.name));
          const validIds = selectedDocuments
            .filter((x) => !failingNames.has(x.name))
            .map((x) => x.id);

          if (validIds.length === 0) {
            Modal.error({
              title: t('knowledgeDetails.parseBlockedTitle'),
              content: buildParserGapModalContent(t, gaps, {
                missingModel: 'knowledgeDetails.addModelToParseHint',
                unsupportedType: 'knowledgeDetails.reselectParserToParseHint',
              }),
              showCancel: false,
              okText: t('common.cancel'),
              closable: false,
            });
            return;
          }

          Modal.warning({
            title: t('knowledgeDetails.parseBlockedPartialTitle'),
            content: (
              <div className="space-y-2">
                {buildParserGapModalContent(t, gaps, {
                  missingModel: 'knowledgeDetails.addModelToParseHint',
                  unsupportedType: 'knowledgeDetails.reselectParserToParseHint',
                })}
                <p>
                  {t('knowledgeDetails.parseValidFilesNote', {
                    count: validIds.length,
                  })}
                </p>
              </div>
            ),
            okText: t('knowledgeDetails.parseValidFiles'),
            cancelText: t('common.cancel'),
            closable: false,
            onOk: async () => {
              await runDocumentByIds({ documentIds: validIds, run, option });
            },
          });
          return;
        }
      }

      await runDocumentByIds({
        documentIds: nonVirtualKeys,
        run,
        option,
      });
      hideModal();
    },
    [
      documents,
      runDocumentByIds,
      selectedRowKeys,
      hideModal,
      t,
      findDocumentParseGaps,
    ],
  );

  const handleRunClick = useCallback(
    (option?: { delete: boolean; apply_kb: boolean }) => {
      runDocument(1, option);
    },
    [runDocument],
  );

  // The confirmation only offers real choices when the selection has existing
  // chunks to drop or auto-metadata to re-apply; otherwise run straight away.
  // Go re-ingests in place server-side, so the dialog is Python-only.
  const needsRunConfirm =
    !isGo &&
    (chunkNum > 0 || Boolean(knowledgeBase?.parser_config?.enable_metadata));

  const handleRunMenuClick = useCallback(() => {
    if (needsRunConfirm) {
      showModal();
      return;
    }
    handleRunClick();
  }, [needsRunConfirm, showModal, handleRunClick]);

  const handleCancelClick = useCallback(() => {
    runDocument(2);
  }, [runDocument]);

  const onChangeStatus = useCallback(
    (enabled: boolean) => {
      setDocumentStatus({
        status: enabled,
        documentId: selectedRowKeys,
        datasetId: id!,
      });
    },
    [selectedRowKeys, setDocumentStatus, id],
  );

  const handleEnableClick = useCallback(() => {
    onChangeStatus(true);
  }, [onChangeStatus]);

  const handleDisableClick = useCallback(() => {
    onChangeStatus(false);
  }, [onChangeStatus]);

  const handleDelete = useCallback(() => {
    const deletedKeys = selectedRowKeys.filter(
      (x) => !documents.filter(isDocumentProcessing).some((y) => y.id === x),
    );
    if (deletedKeys.length === 0) {
      toast.error(
        t('knowledgeConfiguration.theDocumentBeingParsedCannotBeDeleted'),
      );
      return;
    }

    return removeDocument(deletedKeys);
  }, [selectedRowKeys, removeDocument, documents, t]);

  const list = [
    {
      id: 'enabled',
      label: t('knowledgeDetails.enabled'),
      icon: <LucideToggleRight />,
      onClick: handleEnableClick,
    },
    {
      id: 'disabled',
      label: t('knowledgeDetails.disabled'),
      icon: <LucideToggleLeft />,
      onClick: handleDisableClick,
    },
    {
      id: 'run',
      label: t('knowledgeDetails.run'),
      icon: <LucidePlayCircle />,
      onClick: handleRunMenuClick,
    },
    {
      id: 'cancel',
      label: t('knowledgeDetails.cancel'),
      icon: <LucideCircleX />,
      onClick: handleCancelClick,
    },
    {
      id: 'batch-metadata',
      label: t('knowledgeDetails.metadata.metadata'),
      icon: <LucideCylinder />,
    },
    {
      id: 'delete',
      label: t('common.delete'),
      icon: <LucideTrash2 />,
      onClick: async () => {
        const code = await handleDelete();
        if (code === 0) {
          setRowSelection({});
        }
      },
    },
  ];

  return { chunkNum, list, visible, hideModal, showModal, handleRunClick };
}
