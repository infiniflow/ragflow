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

import { HomeIcon } from '../svg-icon';

export enum EmptyType {
  Data = 'data',
  SearchData = 'search-data',
}

export enum EmptyCardType {
  Agent = 'agent',
  Dataset = 'dataset',
  Chat = 'chat',
  Search = 'search',
  Memory = 'memory',
  Skills = 'skills',
}

export const EmptyCardData = {
  [EmptyCardType.Agent]: {
    icon: <HomeIcon name="agents" width={'24'} />,
    titleKey: 'empty.agentTitle',
    notFoundKey: 'empty.notFoundAgent',
  },
  [EmptyCardType.Dataset]: {
    icon: <HomeIcon name="datasets" width={'24'} />,
    titleKey: 'empty.datasetTitle',
    notFoundKey: 'empty.notFoundDataset',
  },
  [EmptyCardType.Chat]: {
    icon: <HomeIcon name="chats" width={'24'} />,
    titleKey: 'empty.chatTitle',
    notFoundKey: 'empty.notFoundChat',
  },
  [EmptyCardType.Search]: {
    icon: <HomeIcon name="searches" width={'24'} />,
    titleKey: 'empty.searchTitle',
    notFoundKey: 'empty.notFoundSearch',
  },
  [EmptyCardType.Memory]: {
    icon: <HomeIcon name="memory" width={'24'} />,
    titleKey: 'empty.memoryTitle',
    notFoundKey: 'empty.notFoundMemory',
  },
  [EmptyCardType.Skills]: {
    icon: <HomeIcon name="skills" width={'24'} />,
    titleKey: 'empty.skillsTitle',
    notFoundKey: 'empty.notFoundSkills',
  },
};
