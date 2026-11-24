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
import { UserSettingCloseButton } from './components/user-setting-close-button';
import { SideBar } from './sidebar';

function UserSetting() {
  return (
    <section className="relative size-full min-w-0 pt-16">
      <UserSettingCloseButton />

      <div className="mx-auto grid size-full max-w-5xl min-w-0 grid-cols-[4rem_minmax(0,1fr)] gap-[clamp(1rem,5vw,5rem)] px-4 pb-6 md:grid-cols-[16rem_minmax(0,1fr)] md:px-8">
        <SideBar />

        <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
          <Outlet />
        </div>
      </div>
    </section>
  );
}

export default UserSetting;
