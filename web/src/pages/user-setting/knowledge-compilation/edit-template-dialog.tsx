import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  useCreateCompilationTemplate,
  useFetchCompilationTemplate,
  useListCompilationTemplates,
  useUpdateCompilationTemplate,
} from '@/hooks/use-compilation-template-request';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { EditTemplateForm } from './edit-template-form';

interface EditTemplateDialogProps {
  id: string;
  hideModal: () => void;
}

/**
 * Wraps {@link EditTemplateForm} in a Dialog and wires create/update
 * mutations. The dialog stays open while the mutation is in-flight; on
 * success it closes and React Query invalidates the list.
 */
export function EditTemplateDialog({ id, hideModal }: EditTemplateDialogProps) {
  const { t } = useTranslation();
  const { data: initial, loading: loadingInitial } =
    useFetchCompilationTemplate(id);
  const { createCompilationTemplate, loading: creating } =
    useCreateCompilationTemplate();
  const { updateCompilationTemplate, loading: updating } =
    useUpdateCompilationTemplate();
  const { data: savedTemplates } = useListCompilationTemplates();

  const isEditing = Boolean(id);
  const loading = loadingInitial || creating || updating;
  const [isDirty, setIsDirty] = useState(false);
  const [confirmCloseVisible, setConfirmCloseVisible] = useState(false);

  const requestClose = useCallback(() => {
    if (isDirty && !loading) {
      setConfirmCloseVisible(true);
      return;
    }
    hideModal();
  }, [hideModal, isDirty, loading]);

  const confirmClose = useCallback(() => {
    setConfirmCloseVisible(false);
    hideModal();
  }, [hideModal]);

  const handleSubmit = useCallback(
    async (values: {
      name: string;
      description?: string;
      kind: any;
      config: any;
    }) => {
      const code = isEditing
        ? await updateCompilationTemplate({ id, ...values })
        : await createCompilationTemplate(values);
      if (code === 0) {
        setIsDirty(false);
        hideModal();
      }
    },
    [
      isEditing,
      id,
      updateCompilationTemplate,
      createCompilationTemplate,
      hideModal,
    ],
  );

  return (
    <>
      <Dialog open onOpenChange={(open) => !open && requestClose()}>
        <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {isEditing
                ? t('knowledgeCompilation.editTemplate')
                : t('knowledgeCompilation.addTemplate')}
            </DialogTitle>
          </DialogHeader>
          {/* When opening for edit, wait for the initial fetch so RHF
            defaultValues are populated correctly. For "add" we render
            immediately with the empty defaults. */}
          {isEditing && loadingInitial ? (
            <p className="p-6 text-sm text-text-secondary">
              {t('common.loading')}
            </p>
          ) : (
            <EditTemplateForm
              initial={isEditing ? initial : undefined}
              savedTemplates={savedTemplates.templates}
              onSubmit={handleSubmit}
              onCancel={requestClose}
              onDirtyChange={setIsDirty}
              loading={loading}
            />
          )}
        </DialogContent>
      </Dialog>
      <AlertDialog
        open={confirmCloseVisible}
        onOpenChange={setConfirmCloseVisible}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('knowledgeCompilation.confirmCloseTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('knowledgeCompilation.confirmCloseBody')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmClose}>
              {t('knowledgeCompilation.discardAndClose')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
