import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const { createSearch, getSearchList, deleteSearch, getSearchDetail } = api;
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
} as const;
const searchService = registerServer<keyof typeof methods>(methods, request);

export default searchService;
