import { DynamicForm, FormFieldConfig } from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { IModalProps } from '@/interfaces/common';
import { useEffect, useMemo, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  ChatChannelFormDefaultValues,
  getChatChannelFields,
  mergeChatChannelFormValues,
} from './constant';
import { IChatChannel, IChatChannelInfo } from './interface';

const AddChatChannelModal = ({
  visible,
  hideModal,
  loading,
  channel,
  record,
  onOk,
}: IModalProps<FieldValues> & {
  channel?: IChatChannelInfo;
  record?: IChatChannel;
}) => {
  const { t } = useTranslation();
  const [fields, setFields] = useState<FormFieldConfig[]>([]);

  useEffect(() => {
    if (channel) {
      setFields(getChatChannelFields(channel.id));
    }
  }, [channel]);

  const defaultValues = useMemo(() => {
    const base = channel ? ChatChannelFormDefaultValues[channel.id] : undefined;
    return mergeChatChannelFormValues(base, record) as FieldValues;
  }, [channel, record]);

  const handleOk = async (values?: FieldValues) => {
    await onOk?.(values);
  };

  return (
    <Modal
      title={
        <div className="flex flex-col gap-4">
          <div className="size-6">{channel?.icon}</div>
          {record
            ? t('setting.editChannelModalTitle', { name: channel?.name })
            : t('setting.addChannelModalTitle', { name: channel?.name })}
        </div>
      }
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      okText={t('common.confirm')}
      cancelText={t('common.cancel')}
      footer={<div className="p-4"></div>}
    >
      <DynamicForm.Root
        fields={fields}
        onSubmit={() => {}}
        defaultValues={defaultValues}
        labelClassName="font-normal"
      >
        <div className="absolute bottom-0 right-0 left-0 flex items-center justify-end w-full gap-2 py-6 px-6">
          <DynamicForm.CancelButton
            handleCancel={() => {
              hideModal?.();
            }}
          />
          <DynamicForm.SavingButton
            submitLoading={loading || false}
            buttonText={t('common.confirm')}
            submitFunc={(values: FieldValues) => {
              handleOk(values);
            }}
          />
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default AddChatChannelModal;
