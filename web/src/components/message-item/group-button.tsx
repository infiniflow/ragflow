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
  // Business rule: when a placeholder assistant bubble is rendered before
  // the first streaming token arrives, the message id is a synthetic
  // client-side string (e.g. "__optimistic_assistant_placeholder__"). It
  // must never reach the feedback / copy / read APIs because the backend
  // has no such row and any feedback request would 404 or, worse, get
  // associated with a real message under a hash collision. Hide the entire
  // toolbar in that case instead of relying on fragile heuristics like
  // `index === 0` (the placeholder is appended with
  // index = derivedMessages.length, which can be any non-zero number).
  isPendingPlaceholder?: boolean;
}

export const AssistantGroupButton = ({
  messageId,
  content,
  prompt,
  audioBinary,
  showLikeButton,
  showLoudspeaker = true,
  isPendingPlaceholder = false,
}: IProps) => {
  // Business rule: the optimistic assistant placeholder carries a synthetic
  // client-side id (e.g. "__optimistic_assistant_placeholder__") that the
  // backend has no record of. Calling the feedback / copy / read APIs with
  // that id would 404 or, worse, collide with a real message. We therefore
  // hide the entire toolbar for the placeholder row. All hooks are still
  // called unconditionally above the early return so we satisfy React's
  // rules of hooks (a no-op is fine — the placeholder row never re-renders
  // here because it is replaced by the real streaming message on the first
  // backend event).
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

  if (isPendingPlaceholder) {
    return null;
  }

  return (
    <>
      <div className="flex gap-1" role="toolbar">
        <CopyToClipboard
          text={content}
          className="border-0"
          size="icon-xs"
          avoidButtonWrapper
        />

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
          <Button
            onClick={showPromptModal}
            variant="transparent"
            size="icon-xs"
            className="border-0"
          >
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
    <div className="flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
      <CopyToClipboard
        text={content}
        className="border-0"
        size="icon-xs"
        avoidButtonWrapper
      />

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
