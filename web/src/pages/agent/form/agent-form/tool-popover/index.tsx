import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { PropsWithChildren } from 'react';
import { ToolCommand } from './tool-command';

export function ToolPopover({ children }: PropsWithChildren) {
  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="w-80 p-0">
        <ToolCommand></ToolCommand>
      </PopoverContent>
    </Popover>
  );
}
