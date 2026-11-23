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

export function RAGFlowLogo({
  onClick,
}: {
  onClick?: React.MouseEventHandler<HTMLHeadingElement>;
}) {
  return (
    <div className="flex items-center gap-4">
      <h1
        onClick={onClick}
        className={cn(
          'bg-gradient-to-l from-[#40EBE3] to-[#4A51FF] bg-clip-text text-4xl font-bold',
        )}
      >
        RAGFlow
      </h1>
    </div>
  );
}
