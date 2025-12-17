import api from '@/utils/api';
import registerServer, { registerNextServer } from '@/utils/register-server';
import request from '@/utils/request';

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
    method: 'post',
  },
  deleteSearch: { url: deleteSearch, method: 'post' },
  getSearchDetail: {
    url: getSearchDetail,
    method: 'get',
  },
  updateSearchSetting: {
    url: updateSearchSetting,
    method: 'post',
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
const searchService = registerServer<keyof typeof methods>(methods, request);
export const searchServiceNext =
  registerNextServer<keyof typeof methods>(methods);

export default searchService;
