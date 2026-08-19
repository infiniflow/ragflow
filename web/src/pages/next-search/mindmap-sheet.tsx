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

import { IndentedTree } from '@/components/indented-tree/indented-tree';
import { Progress } from '@/components/ui/progress';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { usePendingMindMap } from './hooks';

interface IProps extends IModalProps<any> {
  data: any;
}

const MindMapSheet = ({ data, hideModal, loading, visible }: IProps) => {
  const { t } = useTranslation();
  const percent = usePendingMindMap();
  // An empty tree (no children) means the backend honestly found nothing to
  // map. Render an explicit empty state instead of a ghost "root" node.
  const isEmptyMindMap =
    !data || !Array.isArray(data.children) || data.children.length === 0;
  return (
    <Sheet open={visible} modal={false}>
      <SheetContent
        className="top-24 p-0 flex flex-col gap-0 h-auto"
        closeIcon={false}
      >
        <SheetHeader className="border-b py-2 px-4">
          <SheetTitle className="hidden"></SheetTitle>
          <div className="flex w-full justify-between items-center">
            <div className="text-text-primary font-medium text-base">
              {t('chunk.mind')}
            </div>
            <X
              className="text-text-primary cursor-pointer"
              size={16}
              onClick={() => {
                hideModal?.();
              }}
            />
          </div>
        </SheetHeader>
        <div className="flex-1 p-4 overflow-hidden">
          {loading && (
            <div className="rounded-lg w-full h-full">
              <Progress value={percent} className="h-1 flex-1 min-w-10" />
            </div>
          )}
          {!loading && isEmptyMindMap && (
            <div className="bg-bg-card rounded-lg w-full h-full flex items-center justify-center">
              <p className="text-text-secondary">
                {t('knowledgeCompilation.noStructureMindmap')}
              </p>
            </div>
          )}
          {!loading && !isEmptyMindMap && (
            <div className="bg-bg-card rounded-lg w-full h-full">
              <IndentedTree data={data}></IndentedTree>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
};

export default MindMapSheet;
