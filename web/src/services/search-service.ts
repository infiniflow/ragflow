import api from '@/utils/api';
import request from '@/utils/next-request';
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

const shouldPreferLegacySearchApi = __API_PROXY_SCHEME__ !== 'go';

const isSearchRestApi404 = (error: any) => error?.response?.status === 404;
const isSearchRestApi404Response = (response: any) =>
  response?.status === 404 || response?.data?.code === 404;

const withLegacySearchFallback = async <T>(
  restCall: () => Promise<T>,
  legacyCall: () => Promise<T>,
): Promise<T> => {
  try {
    const response = await restCall();
    if (isSearchRestApi404Response(response)) {
      return legacyCall();
    }
    return response;
  } catch (error) {
    if (isSearchRestApi404(error)) {
      return legacyCall();
    }
    throw error;
  }
};

const withPreferredSearchApi = async <T>(
  restCall: () => Promise<T>,
  legacyCall: () => Promise<T>,
): Promise<T> => {
  return shouldPreferLegacySearchApi
    ? withLegacySearchFallback(legacyCall, restCall)
    : withLegacySearchFallback(restCall, legacyCall);
};

export const getSearchListCompat = (params: {
  keywords?: string;
  page_size?: number;
  page?: number;
  orderby?: string;
  desc?: boolean;
  owner_ids?: string[];
}) =>
  withPreferredSearchApi(
    () => searchService.getSearchList({ params }, true),
    () =>
      request.post(api.legacySearchList, {
        params: {
          keywords: params.keywords,
          page_size: params.page_size,
          page: params.page,
          orderby: params.orderby,
          desc: params.desc,
        },
        data: {
          owner_ids: params.owner_ids ?? [],
        },
      }),
  );

export default searchService;
