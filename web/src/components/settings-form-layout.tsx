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

import { cn } from '@/lib/utils';
import type { ComponentPropsWithoutRef, ReactNode, Ref } from 'react';
import { SettingsFormContainer } from './settings-form-container';

type SettingsFormLayoutProps = ComponentPropsWithoutRef<'section'> & {
  children: ReactNode;
  header?: ReactNode;
  actions?: ReactNode;
  scrollContainerRef?: Ref<HTMLDivElement>;
};

export function SettingsFormLayout({
  children,
  header,
  actions,
  scrollContainerRef,
  className,
  ...props
}: SettingsFormLayoutProps) {
  return (
    <SettingsFormContainer
      className={cn(
        'flex min-h-0 flex-1 flex-col overflow-hidden pt-5',
        className,
      )}
      {...props}
    >
      {header && <div className="shrink-0 px-5">{header}</div>}
      <div
        ref={scrollContainerRef}
        className="flex min-h-0 w-full flex-1 flex-col overflow-y-auto px-5 scrollbar-auto"
      >
        {children}
      </div>
      {actions && (
        <footer className="flex w-full shrink-0 items-center justify-end gap-3 p-5 text-right">
          {actions}
        </footer>
      )}
    </SettingsFormContainer>
  );
}
