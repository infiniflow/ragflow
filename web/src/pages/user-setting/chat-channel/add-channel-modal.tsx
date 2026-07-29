import { DynamicForm, FormFieldConfig } from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { IModalProps } from '@/interfaces/common';
import { fetchChatChannelRuntime } from '@/services/chat-channel-service';
import { QrCode } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  ChatChannelFormDefaultValues,
  ChatChannelKey,
  getChatChannelFields,
  getChatChannelRuntimeStatusClass,
  getChatChannelRuntimeStatusText,
  mergeChatChannelFormValues,
} from './constant';
import { IChatChannel, IChatChannelInfo } from './interface';

const getRuntimeSnapshot = (payload: any) =>
  payload?.data?.data ?? payload?.data ?? payload?.runtime ?? payload ?? {};

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
  const [runtimeStatus, setRuntimeStatus] = useState('');
  const [runtimeError, setRuntimeError] = useState('');
  const [runtimeQr, setRuntimeQr] = useState('');
  const runtimePollingInFlightRef = useRef(false);

  useEffect(() => {
    if (channel) {
      setFields(getChatChannelFields(channel.id));
    }
  }, [channel]);

  const refreshRuntime = useCallback(async () => {
    if (channel?.id !== ChatChannelKey.WHATSAPP || !record?.id) {
      return;
    }
    if (runtimePollingInFlightRef.current) {
      return;
    }
    runtimePollingInFlightRef.current = true;
    try {
      const { data } = await fetchChatChannelRuntime(record.id);
      const snapshot = getRuntimeSnapshot(data);
      setRuntimeStatus(snapshot?.status || '');
      setRuntimeError(snapshot?.last_error || '');
      setRuntimeQr(snapshot?.qr_data_url || '');
    } catch (error: any) {
      setRuntimeError(error?.message || 'Failed to load QR.');
      setRuntimeQr('');
    } finally {
      runtimePollingInFlightRef.current = false;
    }
  }, [channel?.id, record?.id]);

  useEffect(() => {
    if (channel?.id !== ChatChannelKey.WHATSAPP || !record?.id) {
      return;
    }
    setRuntimeStatus('');
    setRuntimeError('');
    setRuntimeQr('');
    void refreshRuntime();
    const timer = window.setInterval(() => {
      void refreshRuntime();
    }, 1000);
    return () => window.clearInterval(timer);
  }, [channel?.id, record?.id, refreshRuntime]);

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
        {channel?.id === ChatChannelKey.WHATSAPP && (
          <div className="mb-6 rounded-lg border border-border-button bg-bg-card p-4">
            {record?.id ? (
              <div className="flex flex-col gap-3">
                <div className="flex items-center justify-between gap-2">
                  <div
                    className={`flex items-center gap-2 rounded-full border px-2 py-1 text-sm ${getChatChannelRuntimeStatusClass(runtimeStatus)}`}
                  >
                    <QrCode className="size-4" />
                    <span>
                      {getChatChannelRuntimeStatusText(runtimeStatus)}
                    </span>
                  </div>
                </div>
                {runtimeError ? (
                  <div className="text-sm text-state-error">{runtimeError}</div>
                ) : null}
                {runtimeQr && runtimeStatus !== 'connected' ? (
                  <img
                    src={runtimeQr}
                    alt="WhatsApp QR"
                    className="mx-auto w-56 max-w-full rounded-lg border border-border-button bg-white"
                  />
                ) : runtimeStatus === 'connected' ? (
                  <div className="text-sm text-state-success">
                    Channel is connected.
                  </div>
                ) : !runtimeStatus ? (
                  <div className="text-sm text-text-secondary">
                    QR will appear after the channel starts.
                  </div>
                ) : null}
              </div>
            ) : (
              <div className="text-sm text-text-secondary">
                Save this WhatsApp channel first, then scan the QR code here.
              </div>
            )}
          </div>
        )}
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
