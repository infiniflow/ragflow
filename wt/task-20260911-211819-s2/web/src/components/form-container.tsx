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
import { PropsWithChildren } from 'react';

export type FormContainerProps = {
  className?: string;
  show?: boolean;
} & PropsWithChildren;

export function FormContainer({
  children,
  show = true,
  className,
}: FormContainerProps) {
  return show ? (
    <section
      className={cn(
        'border-0.5 border-border-button rounded-lg p-5 space-y-5',
        className,
      )}
    >
      {children}
    </section>
  ) : (
    children
  );
}
