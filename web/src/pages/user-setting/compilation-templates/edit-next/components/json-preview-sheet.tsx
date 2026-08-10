import { useTranslation } from 'react-i18next';

import JsonEditor from '@/components/json-edit';
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
import { Braces } from 'lucide-react';

interface JsonPreviewSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: Record<string, unknown>;
}

export function JsonPreviewSheet({
  open,
  onOpenChange,
  value,
}: JsonPreviewSheetProps) {
  const { t } = useTranslation();

  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <Tooltip>
        <TooltipTrigger asChild>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <Braces className="size-4" />
            </Button>
          </SheetTrigger>
        </TooltipTrigger>
        <TooltipContent>{t('setting.jsonPreview')}</TooltipContent>
      </Tooltip>
      <SheetContent
        className="w-1/2 max-w-[700px] flex flex-col"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle>{t('setting.jsonPreview')}</SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 mt-4">
          <JsonEditor
            value={value}
            height="100%"
            options={{ mode: 'tree', modes: ['tree', 'code'] }}
            defaultExpanded
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}
