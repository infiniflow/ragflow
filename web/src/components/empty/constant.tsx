import { t } from 'i18next';
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
}

export const EmptyCardData = {
  [EmptyCardType.Agent]: {
    icon: <HomeIcon name="agents" width={'24'} />,
    title: t('empty.agentTitle'),
    notFound: t('empty.notFoundAgent'),
  },
  [EmptyCardType.Dataset]: {
    icon: <HomeIcon name="datasets" width={'24'} />,
    title: t('empty.datasetTitle'),
    notFound: t('empty.notFoundDataset'),
  },
  [EmptyCardType.Chat]: {
    icon: <HomeIcon name="chats" width={'24'} />,
    title: t('empty.chatTitle'),
    notFound: t('empty.notFoundChat'),
  },
  [EmptyCardType.Search]: {
    icon: <HomeIcon name="searches" width={'24'} />,
    title: t('empty.searchTitle'),
    notFound: t('empty.notFoundSearch'),
  },
  [EmptyCardType.Memory]: {
    icon: <HomeIcon name="memory" width={'24'} />,
    title: t('empty.memoryTitle'),
    notFound: t('empty.notFoundMemory'),
  },
};
