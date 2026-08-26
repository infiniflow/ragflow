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

import Spotlight from '@/components/spotlight';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { PropsWithChildren } from 'react';

export function Title({ children }: PropsWithChildren) {
  return <span className="font-bold text-xl">{children}</span>;
}

type ProfileSettingWrapperCardProps = {
  header: React.ReactNode;
} & PropsWithChildren;

export function ProfileSettingWrapperCard({
  header,
  children,
}: ProfileSettingWrapperCardProps) {
  return (
    <Card
      as="article"
      className="relative w-full border-border-button bg-transparent border-0.5 flex flex-col"
    >
      <CardHeader className="flex-0 border-b-0.5 border-border-button p-5">
        {header}
      </CardHeader>

      <CardContent className="flex-1 h-0 p-0">{children}</CardContent>

      <Spotlight />
    </Card>
  );
}
