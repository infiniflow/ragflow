'use client';

import { Form } from '@/components/ui/form';
import { Modal } from '@/components/ui/modal/modal';
import { UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CreateDirectoryFormValues } from '../interface';
import { CreateDirectoryFormFields } from './create-directory-form';

export interface CreateDirectoryDialogProps {
  open: boolean;
  loading: boolean;
  form: UseFormReturn<CreateDirectoryFormValues>;
  onOk: (values: CreateDirectoryFormValues) => void;
  onCancel: () => void;
}

export function CreateDirectoryDialog({
  open,
  loading,
  form,
  onOk,
  onCancel,
}: CreateDirectoryDialogProps) {
  const { t } = useTranslation();

  const handleOk = () => {
    form.handleSubmit(onOk)();
  };

  const handleOpenChange = (val: boolean) => {
    if (!val) {
      onCancel();
    }
  };

  return (
    <Modal
      open={open}
      onOpenChange={handleOpenChange}
      title={t('knowledgeDetails.createDirectoryFolder')}
      onOk={handleOk}
      onCancel={onCancel}
      confirmLoading={loading}
    >
      <Form {...form}>
        <CreateDirectoryFormFields />
      </Form>
    </Modal>
  );
}
