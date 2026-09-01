/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
        <TooltipContent>{t('knowledgeCompilation.processFlow')}</TooltipContent>
      </Tooltip>
      <SheetContent
        className="w-1/2 max-w-[700px] flex flex-col"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle>{t('knowledgeCompilation.processFlow')}</SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 mt-4 flex items-center justify-center">
          <span className="text-text-disabled">
            {t('knowledgeCompilation.processFlowComingSoon')}
          </span>
        </div>
      </SheetContent>
    </Sheet>
  );
}
