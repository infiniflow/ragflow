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
import { useSettingMenuItems } from '@/ee/pages/user-setting/sidebar/use-setting-menu';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { TFunction } from 'i18next';
import { LucideUser } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useHandleMenuClick } from './hooks';

const menuItems = (t: TFunction) => [
  {
    icon: <LucideUser className="size-4" />,
    label: t('setting.profile'),
    key: Routes.Profile,
  },
];

export function SideBar() {
  const { handleMenuClick, active: activeItemKey } = useHandleMenuClick();
  const { t } = useTranslation();
  const items = useSettingMenuItems(menuItems(t));

  return (
    <aside className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-bg-base">
      <h2 className="hidden text-base font-medium text-text-primary md:block">
        {t('setting.settings')}
      </h2>
      <nav className="mt-4 flex-1 overflow-auto py-1">
        <ul className="flex flex-col items-center gap-2 md:items-stretch md:gap-5">
          {items.map((item) => {
            const { key, icon, label, ...rest } = item;

            return (
              <li key={key} className="w-full md:w-auto">
                <Button
                  {...rest}
                  block
                  variant="ghost"
                  aria-label={label}
                  className={cn(
                    'relative h-10 text-base max-md:size-10 max-md:p-0 max-md:justify-center justify-start gap-2.5 px-2 md:px-3',
                    activeItemKey === key && 'bg-bg-card text-text-primary',
                  )}
                  onClick={handleMenuClick(key)}
                >
                  <span className="flex items-center gap-2.5 max-md:gap-0">
                    {icon}
                    <span className="hidden md:inline">{label}</span>
                  </span>
                </Button>
              </li>
            );
          })}
        </ul>
      </nav>
    </aside>
  );
}
