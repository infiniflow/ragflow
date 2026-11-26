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

import { SidebarPageFrame } from '@/components/sidebar-page-frame';
import { Outlet } from 'react-router';
import { UserSettingCloseButton } from './components/user-setting-close-button';
import { SideBar } from './sidebar';

function UserSetting() {
  return (
    <SidebarPageFrame
      sidebar={<SideBar />}
      closeButton={<UserSettingCloseButton />}
    >
      <Outlet />
    </SidebarPageFrame>
  );
}

export default UserSetting;
