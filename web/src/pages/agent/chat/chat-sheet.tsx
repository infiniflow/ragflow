import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { useIsTaskMode } from '../hooks/use-get-begin-query';
import AgentChatBox from './box';

export function ChatSheet({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();
  const isTaskMode = useIsTaskMode();

  return (
    <Sheet open modal={false} onOpenChange={hideModal}>
      <SheetContent
        className={cn('top-20 bottom-0 p-0 flex flex-col h-auto')}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetTitle className="hidden"></SheetTitle>
        <div className="pl-5 pt-2">
          {t(isTaskMode ? 'flow.task' : 'chat.chat')}
        </div>
        <AgentChatBox></AgentChatBox>
      </SheetContent>
    </Sheet>
  );
}
