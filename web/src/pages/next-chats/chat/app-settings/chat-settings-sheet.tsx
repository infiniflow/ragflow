import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { PropsWithChildren } from 'react';
import { ChatSettings } from './chat-settings';

export function ChatSettingSheet({ children }: PropsWithChildren) {
  return (
    <Sheet>
      <SheetTrigger asChild>{children}</SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Chat Settings</SheetTitle>
        </SheetHeader>
        <ChatSettings></ChatSettings>
      </SheetContent>
    </Sheet>
  );
}
