import { PromptIcon } from '@/assets/icon/next-icon';
import CopyToClipboard from '@/components/copy-to-clipboard';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useSetModalState } from '@/hooks/common-hooks';
import { IRemoveMessageById } from '@/hooks/logic-hooks';
import { cn } from '@/lib/utils';
import {
  LucidePauseCircle,
  LucideRefreshCw,
  LucideThumbsDown,
  LucideThumbsUp,
  LucideTrash2,
  LucideVolume2,
} from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import FeedbackDialog from '../feedback-dialog';
import { PromptDialog } from '../prompt-dialog';
import { Button } from '../ui/button';
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
      <div className="flex gap-1" role="toolbar">
        <CopyToClipboard text={content} className="border-0" size="icon-xs" />

        {showLoudspeaker && (
          <>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="transparent"
                  size="icon-xs"
                  className="border-0"
                  onClick={handleRead}
                >
                  <span>
                    {isPlaying ? <LucidePauseCircle /> : <LucideVolume2 />}
                  </span>
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('chat.read')}</TooltipContent>
            </Tooltip>

            <audio src="" ref={ref}></audio>
          </>
        )}
        {showLikeButton && (
          <>
            <Button
              variant="transparent"
              size="icon-xs"
              className="border-0"
              onClick={handleLike}
            >
              <LucideThumbsUp />
            </Button>

            <Button
              variant="transparent"
              size="icon-xs"
              className="border-0"
              onClick={showModal}
            >
              <LucideThumbsDown />
            </Button>
          </>
        )}
        {prompt && (
          <Button onClick={showPromptModal}>
            <PromptIcon style={{ fontSize: '16px' }} />
          </Button>
        )}
      </div>
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
    <div className="flex gap-1">
      <CopyToClipboard text={content} className="border-0" size="icon-xs" />

      {regenerateMessage && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="transparent"
              size="icon-xs"
              className="border-0"
              onClick={regenerateMessage}
              disabled={sendLoading}
            >
              <LucideRefreshCw className={cn(sendLoading && 'animate-spin')} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('chat.regenerate')}</TooltipContent>
        </Tooltip>
      )}
      {removeMessageById && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="transparent"
              size="icon-xs"
              className="border-0"
              onClick={onRemoveMessage}
              disabled={loading}
            >
              <LucideTrash2 />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.delete')}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
};
