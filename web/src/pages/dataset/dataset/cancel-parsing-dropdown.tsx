import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { IngestionTaskStatus } from '@/constants/knowledge';
import {
  useGetDocumentFilter,
  useStopAllDocuments,
} from '@/hooks/use-document-request';
import { Pause, SquarePause } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

enum CancelScope {
  Parsing = 'parsing',
  Pending = 'pending',
}

const CancelScopeStatuses: Record<CancelScope, IngestionTaskStatus[]> = {
  [CancelScope.Parsing]: [IngestionTaskStatus.RUNNING],
  [CancelScope.Pending]: [IngestionTaskStatus.SCHEDULED],
};

function useCancelParsingTasks() {
  const { t } = useTranslation();
  const { id: datasetId } = useParams();
  const { statusCounts } = useGetDocumentFilter();
  const { stopAllDocuments } = useStopAllDocuments();
  const [confirmScope, setConfirmScope] = useState<CancelScope | null>(null);

  const counts: Record<CancelScope, number> = {
    [CancelScope.Parsing]: statusCounts[IngestionTaskStatus.RUNNING] ?? 0,
    [CancelScope.Pending]: statusCounts[IngestionTaskStatus.SCHEDULED] ?? 0,
  };

  const showConfirm = useCallback((scope: CancelScope) => {
    setConfirmScope(scope);
  }, []);

  const handleConfirmOpenChange = useCallback((open: boolean) => {
    if (!open) {
      setConfirmScope(null);
    }
  }, []);

  const handleConfirm = useCallback(async () => {
    if (!confirmScope || !datasetId) {
      return;
    }
    await stopAllDocuments({
      datasetId,
      runStatuses: CancelScopeStatuses[confirmScope],
    });
    setConfirmScope(null);
  }, [confirmScope, datasetId, stopAllDocuments]);

  const confirmContent =
    confirmScope === null
      ? ''
      : t(
          confirmScope === CancelScope.Parsing
            ? 'knowledgeDetails.cancelParsingConfirm'
            : 'knowledgeDetails.cancelPendingConfirm',
          { count: counts[confirmScope] },
        );

  return {
    counts,
    confirmScope,
    showConfirm,
    handleConfirmOpenChange,
    handleConfirm,
    confirmContent,
  };
}

interface CancelScopeItemProps {
  scope: CancelScope;
  label: string;
  disabled: boolean;
  onSelect: (scope: CancelScope) => void;
}

function CancelScopeItem({
  scope,
  label,
  disabled,
  onSelect,
}: CancelScopeItemProps) {
  const handleClick = useCallback(() => {
    onSelect(scope);
  }, [onSelect, scope]);

  return (
    <DropdownMenuItem disabled={disabled} onClick={handleClick}>
      <span>{label}</span>
      <SquarePause className="text-state-error" />
    </DropdownMenuItem>
  );
}

export function CancelParsingDropdown() {
  const { t } = useTranslation();
  const {
    counts,
    confirmScope,
    showConfirm,
    handleConfirmOpenChange,
    handleConfirm,
    confirmContent,
  } = useCancelParsingTasks();

  if (counts[CancelScope.Parsing] + counts[CancelScope.Pending] === 0) {
    return null;
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline">
            <Pause />
            {t('knowledgeDetails.cancelParsingTasks')}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-auto min-w-56">
          <CancelScopeItem
            scope={CancelScope.Parsing}
            label={t('knowledgeDetails.filesParsing', {
              count: counts[CancelScope.Parsing],
            })}
            disabled={counts[CancelScope.Parsing] === 0}
            onSelect={showConfirm}
          />
          <DropdownMenuSeparator />
          <CancelScopeItem
            scope={CancelScope.Pending}
            label={t('knowledgeDetails.filesPending', {
              count: counts[CancelScope.Pending],
            })}
            disabled={counts[CancelScope.Pending] === 0}
            onSelect={showConfirm}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDeleteDialog
        open={confirmScope !== null}
        onOpenChange={handleConfirmOpenChange}
        title={t('knowledgeDetails.cancelParsingConfirmTitle')}
        content={{ title: confirmContent }}
        okButtonText={t('common.ok')}
        onOk={handleConfirm}
      />
    </>
  );
}
