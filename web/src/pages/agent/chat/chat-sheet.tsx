import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { useIsTaskMode } from '../hooks/use-get-begin-query';
import AgentChatBox from './box';

export function ChatSheet({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();
  const isTaskMode = useIsTaskMode();
  // Radix Dialog requires both a Title and a Description on the content
  // node; both are visually hidden so the user-facing header in the
  // separate <div> below stays unchanged.
  const sheetLabel = t(isTaskMode ? 'flow.task' : 'chat.chat');

  return (
    <Sheet open modal={false} onOpenChange={hideModal}>
      <SheetContent
        data-testid="agent-run-chat"
        className={cn('top-20 bottom-0 p-0 flex flex-col h-auto')}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetTitle className="sr-only">{sheetLabel}</SheetTitle>
        <SheetDescription className="sr-only">{sheetLabel}</SheetDescription>
        <div className="pl-5 pt-2">{sheetLabel}</div>
        <AgentChatBox></AgentChatBox>
      </SheetContent>
    </Sheet>
  );
}
