import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import AgentChatBox from './box';

export function ChatSheet({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();
  return (
    <Sheet open modal={false} onOpenChange={hideModal}>
      <SheetContent
        className={cn('top-20 p-0')}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetTitle className="hidden"></SheetTitle>
        <div className="pl-5 pt-2">{t('chat.chat')}</div>
        <AgentChatBox></AgentChatBox>
      </SheetContent>
    </Sheet>
  );
}
