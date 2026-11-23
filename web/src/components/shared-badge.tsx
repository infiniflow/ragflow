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

import { SharedResourceOwnerBadge } from '@/ee/components/shared-resource-owner-badge';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { PropsWithChildren } from 'react';

type SharedBadgeProps = PropsWithChildren<{
  userId?: string;
  ownerNickname?: string;
}>;

export function SharedBadge({
  children,
  userId,
  ownerNickname,
}: SharedBadgeProps) {
  const { data: userInfo } = useFetchUserInfo();

  return (
    <SharedResourceOwnerBadge
      userId={userId}
      currentUserId={userInfo.id}
      currentUserNickname={userInfo.nickname}
      ownerNickname={ownerNickname}
    >
      {children}
    </SharedResourceOwnerBadge>
  );
}
