import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const {
  createSearch,
  getSearchList,
  deleteSearch,
  getSearchDetail,
  updateSearchSetting,
  askShare,
  mindmapShare,
  getRelatedQuestionsShare,
  getSearchDetailShare,
} = api;

const methods = {
  createSearch: {
    url: createSearch,
    method: 'post',
  },
  getSearchList: {
    url: getSearchList,
    method: 'get',
  },
  deleteSearch: { url: deleteSearch, method: 'delete' },
  getSearchDetail: {
    url: getSearchDetail,
    method: 'get',
  },
  updateSearchSetting: {
    url: updateSearchSetting,
    method: 'put',
  },
  askShare: {
    url: askShare,
    method: 'post',
  },
  mindmapShare: {
    url: mindmapShare,
    method: 'post',
  },
  getRelatedQuestionsShare: {
    url: getRelatedQuestionsShare,
    method: 'post',
  },
  getSearchDetailShare: {
    url: getSearchDetailShare,
    method: 'get',
  },
} as const;

const searchService = registerNextServer<keyof typeof methods>(methods);
export const searchServiceNext = searchService;

export default searchService;
