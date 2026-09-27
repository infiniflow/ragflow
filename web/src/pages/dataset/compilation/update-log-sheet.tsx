import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { ITraceInfo } from '@/hooks/use-dataset-generate';

import { ProgressLogPanel } from './progress-log-panel';

type UpdateLogSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  data?: ITraceInfo;
  title: string;
};

function preventOutsideDismiss(e: Event) {
  e.preventDefault();
}

export function UpdateLogSheet({
  open,
  onOpenChange,
  data,
  title,
}: UpdateLogSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent
        className="flex flex-col"
        onInteractOutside={preventOutsideDismiss}
      >
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 flex flex-col pt-4">
          <ProgressLogPanel
            progressMsg={data?.progress_msg}
            className="flex-1"
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}
