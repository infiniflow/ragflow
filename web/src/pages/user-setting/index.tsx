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

import { Button } from '@/components/ui/button';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { cn } from '@/lib/utils';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

function UserSetting() {
  const { t } = useTranslation();
  const { closeUserSetting } = useNavigatePage();

  return (
    <section className="relative pt-16 size-full grid grid-cols-[4rem_minmax(0,1fr)] md:grid-cols-[303px_minmax(0,1fr)] grid-rows-1 min-w-0">
      <Button
        variant="ghost"
        size="icon"
        aria-label={t('common.close')}
        title={t('common.close')}
        onClick={closeUserSetting}
        className="absolute right-2 top-4 z-10 md:right-6"
      >
        <X className="size-6" />
      </Button>

      <SideBar />

      <div
        className={cn(
          'pr-16 md:pr-20 pb-6 flex flex-1 min-w-0 rounded-lg overflow-hidden',
        )}
      >
        <Outlet />
      </div>
    </section>
  );
}

export default UserSetting;
