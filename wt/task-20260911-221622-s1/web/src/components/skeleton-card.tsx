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

import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

type SkeletonCardProps = {
  className?: string;
};
export function SkeletonCard(props: SkeletonCardProps) {
  const { className } = props;
  return (
    <div className={cn('space-y-4', className)}>
      <Skeleton className="h-8 w-full bg-bg-card rounded-lg" />
      <Skeleton className="h-8 w-4/5 bg-bg-card rounded-lg" />
      <Skeleton className="h-8 w-3/5 bg-bg-card rounded-lg" />
    </div>
  );
}
