import { PromptIcon } from '@/assets/icon/next-icon';
import CopyToClipboard from '@/components/copy-to-clipboard';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useSetModalState } from '@/hooks/common-hooks';
import { IRemoveMessageById } from '@/hooks/logic-hooks';
import {
  DeleteOutlined,
  DislikeOutlined,
  LikeOutlined,
  PauseCircleOutlined,
  SoundOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import FeedbackDialog from '../feedback-dialog';
import { PromptDialog } from '../prompt-dialog';
import { useRemoveMessage, useSendFeedback, useSpeech } from './hooks';

interface IProps {
  messageId: string;
  content: string;
  prompt?: string;
  showLikeButton: boolean;
  audioBinary?: string;
  showLoudspeaker?: boolean;
}

export const AssistantGroupButton = ({
  messageId,
  content,
  prompt,
  audioBinary,
  showLikeButton,
  showLoudspeaker = true,
}: IProps) => {
  const { visible, hideModal, showModal, onFeedbackOk, loading } =
    useSendFeedback(messageId);
  const {
    visible: promptVisible,
    hideModal: hidePromptModal,
    showModal: showPromptModal,
  } = useSetModalState();
  const { t } = useTranslation();
  const { handleRead, ref, isPlaying } = useSpeech(content, audioBinary);

  const handleLike = useCallback(() => {
    onFeedbackOk({ thumbup: true });
  }, [onFeedbackOk]);

  return (
    <>
      <ToggleGroup
        type="single"
        size="sm"
        variant="outline"
        className="space-x-1"
      >
        <ToggleGroupItem value="a">
          <CopyToClipboard text={content}></CopyToClipboard>
        </ToggleGroupItem>
        {showLoudspeaker && (
          <ToggleGroupItem value="b" onClick={handleRead}>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  {isPlaying ? <PauseCircleOutlined /> : <SoundOutlined />}
                </span>
              </TooltipTrigger>
              <TooltipContent>{t('chat.read')}</TooltipContent>
            </Tooltip>
            <audio src="" ref={ref}></audio>
          </ToggleGroupItem>
        )}
        {showLikeButton && (
          <>
            <ToggleGroupItem value="c" onClick={handleLike}>
              <LikeOutlined />
            </ToggleGroupItem>
            <ToggleGroupItem value="d" onClick={showModal}>
              <DislikeOutlined />
            </ToggleGroupItem>
          </>
        )}
        {prompt && (
          <ToggleGroupItem value="e" onClick={showPromptModal}>
            <PromptIcon style={{ fontSize: '16px' }} />
          </ToggleGroupItem>
        )}
      </ToggleGroup>
      {visible && (
        <FeedbackDialog
          visible={visible}
          hideModal={hideModal}
          onOk={onFeedbackOk}
          loading={loading}
        ></FeedbackDialog>
      )}
      {promptVisible && (
        <PromptDialog
          visible={promptVisible}
          hideModal={hidePromptModal}
          prompt={prompt}
        ></PromptDialog>
      )}
    </>
  );
};

interface UserGroupButtonProps extends Partial<IRemoveMessageById> {
  messageId: string;
  content: string;
  regenerateMessage?: () => void;
  sendLoading: boolean;
}

export const UserGroupButton = ({
  content,
  messageId,
  sendLoading,
  removeMessageById,
  regenerateMessage,
}: UserGroupButtonProps) => {
  const { onRemoveMessage, loading } = useRemoveMessage(
    messageId,
    removeMessageById,
  );
  const { t } = useTranslation();

  return (
    <ToggleGroup
      type="single"
      size="sm"
      variant="outline"
      className="space-x-1"
    >
      <ToggleGroupItem value="a">
        <CopyToClipboard text={content}></CopyToClipboard>
      </ToggleGroupItem>
      {regenerateMessage && (
        <ToggleGroupItem
          value="b"
          onClick={regenerateMessage}
          disabled={sendLoading}
        >
          <Tooltip>
            <TooltipTrigger asChild>
              <SyncOutlined spin={sendLoading} />
            </TooltipTrigger>
            <TooltipContent>{t('chat.regenerate')}</TooltipContent>
          </Tooltip>
        </ToggleGroupItem>
      )}
      {removeMessageById && (
        <ToggleGroupItem value="c" onClick={onRemoveMessage} disabled={loading}>
          <Tooltip>
            <TooltipTrigger asChild>
              <DeleteOutlined spin={loading} />
            </TooltipTrigger>
            <TooltipContent>{t('common.delete')}</TooltipContent>
          </Tooltip>
        </ToggleGroupItem>
      )}
    </ToggleGroup>
  );
};
