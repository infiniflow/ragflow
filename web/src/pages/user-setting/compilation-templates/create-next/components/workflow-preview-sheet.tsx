import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Workflow } from 'lucide-react';

interface WorkflowPreviewSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function WorkflowPreviewSheet({
  open,
  onOpenChange,
}: WorkflowPreviewSheetProps) {
  const { t } = useTranslation();

  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <Tooltip>
        <TooltipTrigger asChild>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <Workflow className="size-4" />
            </Button>
          </SheetTrigger>
        </TooltipTrigger>
        <TooltipContent>{t('setting.processFlow')}</TooltipContent>
      </Tooltip>
      <SheetContent
        className="w-1/2 max-w-[700px] flex flex-col"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle>{t('setting.processFlow')}</SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 mt-4 flex items-center justify-center">
          <span className="text-text-disabled">
            {t('setting.processFlowComingSoon')}
          </span>
        </div>
      </SheetContent>
    </Sheet>
  );
}
