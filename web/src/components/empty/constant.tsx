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
