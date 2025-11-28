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
}

export const EmptyCardData = {
  [EmptyCardType.Agent]: {
    icon: <HomeIcon name="agents" width={'24'} />,
    title: t('empty.agentTitle'),
  },
  [EmptyCardType.Dataset]: {
    icon: <HomeIcon name="datasets" width={'24'} />,
    title: t('empty.datasetTitle'),
  },
  [EmptyCardType.Chat]: {
    icon: <HomeIcon name="chats" width={'24'} />,
    title: t('empty.chatTitle'),
  },
  [EmptyCardType.Search]: {
    icon: <HomeIcon name="searches" width={'24'} />,
    title: t('empty.searchTitle'),
  },
};
