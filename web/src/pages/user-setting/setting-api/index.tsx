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

import ApiContent from '@/components/api-service/chat-overview-modal/api-content';
import { UserSettingCloseButton } from '../components/user-setting-close-button';

const ApiPage = () => {
  return (
    <section className="relative size-full min-w-0 pt-16">
      <UserSettingCloseButton />
      <div className="flex size-full min-h-0 min-w-0 overflow-hidden px-4 pb-6 md:px-8">
        <ApiContent idKey="dialogId" />
      </div>
    </section>
  );
};

export default ApiPage;
