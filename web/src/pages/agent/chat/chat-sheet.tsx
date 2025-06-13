import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import AgentChatBox from './box';

export function ChatSheet({ hideModal }: IModalProps<any>) {
  return (
    <Sheet open modal={false} onOpenChange={hideModal}>
      <SheetTitle className="hidden"></SheetTitle>
      <SheetContent
        className={cn('top-20 p-0')}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle>Are you absolutely sure?</SheetTitle>
        </SheetHeader>
        <AgentChatBox></AgentChatBox>
      </SheetContent>
    </Sheet>
  );
}
