import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form } from '@/components/ui/form';
import { Modal } from '@/components/ui/modal/modal';
import { Textarea } from '@/components/ui/textarea';
import { UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CommitFormValues } from './interface';

type WikiCommitModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  form: UseFormReturn<CommitFormValues>;
  onConfirm: (values: CommitFormValues) => void;
  loading?: boolean;
};

export function WikiCommitModal({
  open,
  onOpenChange,
  form,
  onConfirm,
  loading,
}: WikiCommitModalProps) {
  const { t } = useTranslation();

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={t('knowledgeDetails.confirmCommit')}
      onOk={() => form.handleSubmit(onConfirm)()}
      confirmLoading={loading}
      okText={t('common.confirm')}
      cancelText={t('common.cancel')}
    >
      <Form {...form}>
        <form className="space-y-4">
          <RAGFlowFormItem
            name="comments"
            label={t('knowledgeDetails.versionContent')}
            required
            rules={{ required: true }}
          >
            {(field) => (
              <Textarea
                placeholder={t('knowledgeDetails.versionContentPlaceholder')}
                {...field}
                autoSize={{ minRows: 4, maxRows: 8 }}
              />
            )}
          </RAGFlowFormItem>
        </form>
      </Form>
    </Modal>
  );
}
