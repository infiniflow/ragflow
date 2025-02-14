import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useNavigate, useSearchParams } from 'umi';

export enum QueryStringMap {
  KnowledgeId = 'knowledgeId',
}

export const useNavigatePage = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const navigateToDatasetList = useCallback(() => {
    navigate(Routes.Datasets);
  }, [navigate]);

  const navigateToDataset = useCallback(
    (id: string) => () => {
      navigate(`${Routes.Dataset}/${id}`);
    },
    [navigate],
  );

  const navigateToHome = useCallback(() => {
    navigate(Routes.Home);
  }, [navigate]);

  const navigateToProfile = useCallback(() => {
    navigate(Routes.ProfileSetting);
  }, [navigate]);

  const navigateToChatList = useCallback(() => {
    navigate(Routes.Chats);
  }, [navigate]);

  const navigateToChat = useCallback(() => {
    navigate(Routes.Chat);
  }, [navigate]);

  const navigateToChunkParsedResult = useCallback(
    (id: string, knowledgeId?: string) => () => {
      navigate(
        `${Routes.ParsedResult}/${id}?${QueryStringMap.KnowledgeId}=${knowledgeId}`,
      );
    },
    [navigate],
  );

  const getQueryString = useCallback(
    (queryStringKey?: QueryStringMap) => {
      const allQueryString = {
        [QueryStringMap.KnowledgeId]: searchParams.get(
          QueryStringMap.KnowledgeId,
        ),
      };
      if (queryStringKey) {
        return allQueryString[queryStringKey];
      }
      return allQueryString;
    },
    [searchParams],
  );

  return {
    navigateToDatasetList,
    navigateToDataset,
    navigateToHome,
    navigateToProfile,
    navigateToChatList,
    navigateToChat,
    navigateToChunkParsedResult,
    getQueryString,
  };
};
