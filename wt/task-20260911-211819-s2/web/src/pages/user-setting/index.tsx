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

import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

import { cn } from '@/lib/utils';

function UserSetting() {
  return (
    <section className="pt-8 size-full grid grid-cols-[4rem_minmax(0,1fr)] md:grid-cols-[303px_minmax(0,1fr)] grid-rows-1 min-w-0">
      <SideBar />

      <div
        className={cn(
          'pr-2 md:pr-6 pb-6 flex flex-1 min-w-0 rounded-lg overflow-hidden',
        )}
      >
        <Outlet />
      </div>
    </section>
  );
}

export default UserSetting;
