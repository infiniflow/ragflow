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

import { useFetchAppConf } from '@/hooks/logic-hooks';
import { RefreshCcw } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { RAGFlowAvatar } from './ragflow-avatar';
import { Button } from './ui/button';

type EmbedContainerProps = {
  title: string;
  avatar?: string;
  handleReset?(): void;
  hideReset?: boolean;
} & PropsWithChildren;

export function EmbedContainer({
  title,
  avatar,
  children,
  handleReset,
  hideReset = false,
}: EmbedContainerProps) {
  const appConf = useFetchAppConf();

  return (
    <section className="h-[100vh] flex justify-center items-center">
      <div className="w-full h-full md:w-[80vw] md:h-auto border-0 md:border rounded-none md:rounded-lg">
        <div className="flex justify-between items-center border-b p-3 relative">
          <div className="flex gap-2 items-center absolute left-1/2 -translate-x-1/2 md:static md:left-auto md:translate-x-0">
            <RAGFlowAvatar
              avatar={avatar}
              name={title}
              isPerson
              className="size-5 md:size-10"
            />
            <div className="md:text-xl text-foreground">{title}</div>
          </div>
          <div className="flex items-center gap-2 md:ml-auto md:mr-3">
            <img src="/logo.svg" alt="" className="h-6 md:h-8" />
            <span className="hidden md:inline-block text-lg font-bold text-foreground">
              {appConf.appName}
            </span>
          </div>
          {hideReset || (
            <Button
              variant={'secondary'}
              className="text-sm text-foreground cursor-pointer"
              onClick={handleReset}
            >
              <div className="flex gap-1 items-center">
                <RefreshCcw size={14} />
                <span className="hidden text-lg md:inline-block">Reset</span>
              </div>
            </Button>
          )}
        </div>
        {children}
      </div>
    </section>
  );
}
