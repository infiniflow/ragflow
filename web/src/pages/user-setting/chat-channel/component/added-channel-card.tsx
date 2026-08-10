import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Modal } from '@/components/ui/modal/modal';
import { useSetModalState } from '@/hooks/common-hooks';
import { fetchChatChannelRuntime } from '@/services/chat-channel-service';
import { Link2, QrCode, RefreshCw, Settings, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ChatChannelKey,
  getChatChannelRuntimeStatusClass,
  getChatChannelRuntimeStatusText,
  useChatChannelInfo,
} from '../constant';
import { useDeleteChatChannel, useFetchChatChannelDetail } from '../hooks';
import { IChatChannel, IChatChannelBase, IChatChannelInfo } from '../interface';
import ConnectDialogModal from './connect-dialog-modal';
import { delChannelModal } from './delete-channel-modal';

const getRuntimeSnapshot = (payload: any) =>
  payload?.data?.data ?? payload?.data ?? payload?.runtime ?? payload ?? {};

export type IAddedChannelCardProps = IChatChannelInfo & {
  list: IChatChannelBase[];
  onEdit: (channel: IChatChannelInfo, record: IChatChannel) => void;
};

export const AddedChannelCard = (props: IAddedChannelCardProps) => {
  const { list, name, icon, onEdit, ...channel } = props;
  const { t } = useTranslation();
  const { handleDelete } = useDeleteChatChannel();
  const { fetchDetail } = useFetchChatChannelDetail();
  const { chatChannelInfo } = useChatChannelInfo();
  const {
    visible: connectVisible,
    showModal: showConnectModal,
    hideModal: hideConnectModal,
  } = useSetModalState();
  const [connectTarget, setConnectTarget] = useState<
    IChatChannelBase | undefined
  >(undefined);
  const [qrVisible, setQrVisible] = useState(false);
  const [qrChannelId, setQrChannelId] = useState<string>('');
  const [qrLoading, setQrLoading] = useState(false);
  const [qrData, setQrData] = useState<string>('');
  const [qrStatus, setQrStatus] = useState<string>('');
  const [qrError, setQrError] = useState<string>('');
  const qrPollingInFlightRef = useRef(false);

  const handleEdit = async (id: string) => {
    const record = await fetchDetail(id);
    if (record) {
      onEdit({ name, icon, ...channel }, record);
    }
  };

  const handleConnect = (item: IChatChannelBase) => {
    setConnectTarget(item);
    showConnectModal();
  };

  const loadQr = useCallback(async () => {
    if (!qrChannelId) {
      return;
    }
    if (qrPollingInFlightRef.current) {
      return;
    }
    qrPollingInFlightRef.current = true;
    setQrLoading(true);
    try {
      const { data } = await fetchChatChannelRuntime(qrChannelId);
      const snapshot = getRuntimeSnapshot(data);
      setQrData(snapshot?.qr_data_url || '');
      setQrStatus(snapshot?.status || '');
      setQrError(snapshot?.last_error || '');
    } catch (error: any) {
      setQrData('');
      setQrStatus('');
      setQrError(error?.message || 'Failed to load QR.');
    } finally {
      qrPollingInFlightRef.current = false;
      setQrLoading(false);
    }
  }, [qrChannelId]);

  useEffect(() => {
    if (!qrVisible) {
      return;
    }
    void loadQr();
    const timer = window.setInterval(() => {
      void loadQr();
    }, 1000);
    return () => window.clearInterval(timer);
  }, [qrVisible, loadQr]);

  return (
    <Card className="bg-transparent border border-border-button px-5 pt-[10px] pb-5 rounded-md">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-3">
        <CardTitle className="text-base items-center flex gap-1 font-normal">
          {icon}
          {name}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-2 flex flex-col gap-2">
        {list.map((item) => (
          <div
            key={item.id}
            className="flex flex-row items-center justify-between rounded-md bg-bg-card px-[10px] py-4"
          >
            <div className="flex flex-col gap-0.5">
              <div className="text-sm text-text-primary">{item.name}</div>
              {item.chat_id ? (
                <div className="text-xs text-text-secondary flex items-center gap-1">
                  <Link2 size={12} />
                  {item.dialog_name || item.chat_id}
                </div>
              ) : (
                <div className="text-xs text-text-secondary/60">
                  {t('setting.notConnected')}
                </div>
              )}
            </div>
            <div className="text-sm text-text-secondary flex gap-2">
              <Button
                variant={'ghost'}
                className="rounded-lg px-2 py-1 bg-transparent hover:bg-bg-card"
                onClick={() => handleConnect(item)}
                title={t('setting.connectDialog')}
              >
                <Link2 size={14} />
              </Button>
              <Button
                variant={'ghost'}
                className="rounded-lg px-2 py-1 bg-transparent hover:bg-bg-card"
                onClick={() => handleEdit(item.id)}
              >
                <Settings size={14} />
              </Button>
              {channel.channel === ChatChannelKey.WHATSAPP && (
                <Button
                  variant={'ghost'}
                  className="rounded-lg px-2 py-1 bg-transparent hover:bg-bg-card"
                  onClick={() => {
                    setQrChannelId(item.id);
                    setQrData('');
                    setQrStatus('');
                    setQrError('');
                    setQrVisible(true);
                  }}
                  title="Show QR"
                >
                  <QrCode size={14} />
                </Button>
              )}
              <Button
                variant={'ghost'}
                className="rounded-lg px-2 py-1 bg-transparent hover:bg-state-error-5 hover:text-state-error"
                onClick={() =>
                  delChannelModal({
                    data: item,
                    chatChannelInfo,
                    onOk: () => {
                      handleDelete(item.id);
                    },
                  })
                }
              >
                <Trash2 className="cursor-pointer" size={14} />
              </Button>
            </div>
          </div>
        ))}
      </CardContent>
      {connectVisible && (
        <ConnectDialogModal
          visible={connectVisible}
          hideModal={hideConnectModal}
          channel={connectTarget}
        />
      )}
      <Modal
        open={qrVisible}
        onOpenChange={(open) => !open && setQrVisible(false)}
        title="WhatsApp QR Code"
        size="large"
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => void loadQr()}>
              <RefreshCw className={qrLoading ? 'animate-spin' : ''} />
              Refresh
            </Button>
            <Button onClick={() => setQrVisible(false)}>
              {t('common.close')}
            </Button>
          </div>
        }
      >
        <div className="flex flex-col gap-3">
          <div
            className={`inline-flex items-center gap-2 rounded-full border px-2 py-1 text-sm ${getChatChannelRuntimeStatusClass(qrStatus)}`}
          >
            <QrCode className="size-4" />
            <span>{getChatChannelRuntimeStatusText(qrStatus)}</span>
          </div>
          {qrError ? (
            <div className="text-sm text-state-error">{qrError}</div>
          ) : null}
          {qrData ? (
            <img
              src={qrData}
              alt="WhatsApp QR"
              className="mx-auto w-56 max-w-full rounded-lg border border-border-button bg-white"
            />
          ) : (
            <div className="text-sm text-text-secondary">
              QR is not ready yet.
            </div>
          )}
        </div>
      </Modal>
    </Card>
  );
};
