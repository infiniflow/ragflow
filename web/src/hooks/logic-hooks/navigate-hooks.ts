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

import { AgentCategory, AgentQuery } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';
import { NavigateToDataflowResultProps } from '@/pages/dataflow-result/interface';
import { Routes } from '@/routes';
import { useCallback } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router';

export enum QueryStringMap {
  KnowledgeId = 'knowledgeId',
  id = 'id',
}

type UserSettingLocationState = {
  returnTo?: string;
};

type AgentExploreNavigationOptions = {
  sessionId?: string;
  isNew?: boolean;
};

export const useNavigatePage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const returnTo = (location.state as UserSettingLocationState | null)
    ?.returnTo;

  const navigateToDatasetList = useCallback(
    ({ isCreate = false }: { isCreate?: boolean }) => {
      if (isCreate) {
        navigate(Routes.Datasets + '?isCreate=true');
      } else {
        navigate(Routes.Datasets);
      }
    },
    [navigate],
  );

  const navigateToMemoryList = useCallback(
    ({ isCreate = false }: { isCreate?: boolean }) => {
      if (isCreate) {
        navigate(Routes.Memories + '?isCreate=true');
      } else {
        navigate(Routes.Memories);
      }
    },
    [navigate],
  );

  const navigateToApplicationList = useCallback(
    (route: Routes, { isCreate = false }: { isCreate?: boolean } = {}) =>
      () => {
        navigate(isCreate ? `${route}?isCreate=true` : route);
      },
    [navigate],
  );

  const navigateToDataset = useCallback(
    (id: string) => () => {
      // navigate(`${Routes.DatasetBase}${Routes.DataSetOverview}/${id}`);
      navigate(`${Routes.Dataset}/${id}`);
    },
    [navigate],
  );
  const navigateToDatasetSection = useCallback(
    (id: string | undefined, section: Routes, state?: unknown) => () => {
      navigate(`${Routes.DatasetBase}${section}/${id}`, { state });
    },
    [navigate],
  );

  const navigateToDatasetOverview = useCallback(
    (id: string) => () => {
      navigate(`${Routes.DatasetBase}${Routes.DataSetOverview}/${id}`);
    },
    [navigate],
  );

  const navigateToDataFile = useCallback(
    (id: string) => () => {
      navigate(`${Routes.DatasetBase}${Routes.Files}/${id}`);
    },
    [navigate],
  );

  const navigateToDatasetSetting = useCallback(
    (id: string) => {
      navigate(`${Routes.DatasetBase}${Routes.DataSetSetting}/${id}`);
    },
    [navigate],
  );

  const navigateToCompilation = useCallback(
    (id: string) => () => {
      navigate(`${Routes.DatasetBase}${Routes.Compilation}/${id}`);
    },
    [navigate],
  );

  const navigateToHome = useCallback(() => {
    navigate(Routes.Root);
  }, [navigate]);

  const navigateToDefaultPage = useCallback(
    (pathname: string) => () => {
      navigate(pathname.startsWith(Routes.Admin) ? Routes.Admin : Routes.Root);
    },
    [navigate],
  );

  const navigateToProfile = useCallback(() => {
    navigate(Routes.ProfileSetting);
  }, [navigate]);

  const navigateToUserSetting = useCallback(
    (section: string) => {
      const isUserSettingPath =
        location.pathname === Routes.UserSetting ||
        location.pathname.startsWith(`${Routes.UserSetting}/`);
      const currentPath = `${location.pathname}${location.search}${location.hash}`;

      navigate(`${Routes.UserSetting}${section}`, {
        state: { returnTo: isUserSettingPath ? returnTo : currentPath },
      });
    },
    [location, navigate, returnTo],
  );

  const navigateToUserSettingProfile = useCallback(() => {
    navigateToUserSetting(Routes.Profile);
  }, [navigateToUserSetting]);

  const navigateToUserSettingApi = useCallback(() => {
    navigateToUserSetting(Routes.Api);
  }, [navigateToUserSetting]);

  const closeUserSetting = useCallback(() => {
    navigate(returnTo || Routes.Datasets, { replace: true });
  }, [navigate, returnTo]);

  const navigateToChatList = useCallback(() => {
    navigate(Routes.Chats);
  }, [navigate]);

  const navigateToChat = useCallback(
    (id: string) => () => {
      navigate(`${Routes.Chat}/${id}`);
    },
    [navigate],
  );

  // The agents list selects its tab from the canvas_category query param, so
  // callers back on a canvas page pass their own category to land on the
  // matching tab instead of the default workflow tab.
  const navigateToAgents = useCallback(
    (canvasCategory?: AgentCategory | AgentListItemType) => {
      navigate(
        canvasCategory
          ? `${Routes.Agents}?${AgentQuery.CanvasCategory}=${canvasCategory}`
          : Routes.Agents,
      );
    },
    [navigate],
  );

  const navigateToAgentsFromCompilation = useCallback(
    (source: string | null) => () => {
      navigateToAgents(
        source === 'agents'
          ? AgentListItemType.CompilationTemplateGroup
          : undefined,
      );
    },
    [navigateToAgents],
  );

  const navigateToAgentList = useCallback(() => {
    navigate(Routes.AgentList);
  }, [navigate]);

  const navigateToAgent = useCallback(
    (id: string, category?: AgentCategory) => () => {
      navigate(`${Routes.Agent}/${id}?${AgentQuery.Category}=${category}`);
    },
    [navigate],
  );

  const navigateToAgentExplore = useCallback(
    (
      canvasId: string | undefined,
      { sessionId, isNew }: AgentExploreNavigationOptions = {},
    ) => {
      const params = new URLSearchParams();
      if (sessionId) {
        params.set('sessionId', sessionId);
      }
      if (isNew) {
        params.set('isNew', 'true');
      }
      const queryString = params.toString();
      navigate(
        `${Routes.Agent}/${canvasId}${Routes.Explore}${queryString ? `?${queryString}` : ''}`,
      );
    },
    [navigate],
  );

  // Built-in pipelines open on the dedicated read-only canvas, not on the
  // editing canvas. The dataflow_canvas category query keeps the pipeline
  // rendering semantics (handle topology, node forms) identical to editing.
  const navigateToBuiltinPipeline = useCallback(
    (id: string) => () => {
      navigate(
        `${Routes.Pipeline}/${id}?${AgentQuery.Category}=${AgentCategory.DataflowCanvas}`,
      );
    },
    [navigate],
  );

  const navigateToAgentLogs = useCallback(
    (id: string) => () => {
      navigate(`${Routes.AgentLogPage}/${id}`);
    },
    [navigate],
  );

  const navigateToAgentTemplates = useCallback(
    (canvasCategory: string) => {
      const params = new URLSearchParams({
        [AgentQuery.CanvasCategory]: canvasCategory,
      });
      navigate(`${Routes.AgentTemplates}?${params.toString()}`);
    },
    [navigate],
  );

  const navigateToCompilationEditor = useCallback(
    (id?: string) => {
      const params = new URLSearchParams({
        source: 'agents',
        [AgentQuery.CanvasCategory]: AgentListItemType.CompilationTemplateGroup,
      });
      const path = id
        ? `${Routes.CompilationTemplatesEditNext}/${id}`
        : Routes.CompilationTemplatesEditNext;
      navigate(`${path}?${params.toString()}`);
    },
    [navigate],
  );

  const navigateToSearchList = useCallback(() => {
    navigate(Routes.Searches);
  }, [navigate]);

  const navigateToSearch = useCallback(
    (id: string) => () => {
      navigate(`${Routes.Search}/${id}`);
    },
    [navigate],
  );
  const navigateToMemory = useCallback(
    (id: string) => () => {
      navigate(`${Routes.Memory}${Routes.MemoryMessage}/${id}`);
    },
    [navigate],
  );

  const navigateToMemorySection = useCallback(
    (id: string | undefined, section: Routes) => () => {
      navigate(`${Routes.Memory}${section}/${id}`);
    },
    [navigate],
  );

  const navigateToChunkParsedResult = useCallback(
    (id: string, knowledgeId?: string) => () => {
      navigate(
        `${Routes.ParsedResult}/chunks?id=${knowledgeId}&doc_id=${id}`,
        // `${Routes.DataflowResult}?id=${knowledgeId}&doc_id=${id}&type=chunk`,
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
        [QueryStringMap.id]: searchParams.get(QueryStringMap.id),
      };
      if (queryStringKey) {
        return allQueryString[queryStringKey];
      }
      return allQueryString;
    },
    [searchParams],
  );

  const navigateToFiles = useCallback(
    (folderId?: string) => {
      const params = new URLSearchParams();
      if (folderId) {
        params.set('folderId', folderId);
      }
      const queryString = params.toString();
      navigate(`${Routes.Files}${queryString ? `?${queryString}` : ''}`);
    },
    [navigate],
  );

  const navigateToFolder = useCallback(
    (folderId?: string) => () => {
      navigateToFiles(folderId);
    },
    [navigateToFiles],
  );

  const navigateToSkills = useCallback(() => {
    navigate(Routes.Skills);
  }, [navigate]);

  const navigateToDataSourceDetail = useCallback(
    (id?: string) => {
      navigate(
        `${Routes.Integrations}${Routes.DataSource}${Routes.DataSourceDetailPage}?id=${id}`,
      );
    },
    [navigate],
  );

  const navigateToDataflowResult = useCallback(
    (props: NavigateToDataflowResultProps) => () => {
      const params: string[] = [];
      Object.keys(props).forEach((key) => {
        if (props[key as keyof typeof props]) {
          // Values may contain characters like `&`, `=` or `#` (e.g. file
          // names), which would corrupt the query string without encoding.
          params.push(
            `${key}=${encodeURIComponent(props[key as keyof typeof props] as string)}`,
          );
        }
      });
      navigate(
        // `${Routes.ParsedResult}/${id}?${QueryStringMap.KnowledgeId}=${knowledgeId}`,
        `${Routes.DataflowResult}?${params.join('&')}`,
      );
    },
    [navigate],
  );

  const navigateToModelSetting = useCallback(() => {
    navigate(`${Routes.Integrations}${Routes.Model}`);
  }, [navigate]);

  return {
    navigateToDatasetList,
    navigateToDataset,
    navigateToDatasetSection,
    navigateToDatasetOverview,
    navigateToHome,
    navigateToDefaultPage,
    navigateToProfile,
    navigateToChatList,
    navigateToChat,
    navigateToChunkParsedResult,
    getQueryString,
    navigateToAgents,
    navigateToAgentsFromCompilation,
    navigateToAgent,
    navigateToAgentExplore,
    navigateToAgentLogs,
    navigateToAgentTemplates,
    navigateToCompilationEditor,
    navigateToBuiltinPipeline,
    navigateToSearchList,
    navigateToSearch,
    navigateToFiles,
    navigateToFolder,
    navigateToSkills,
    navigateToAgentList,
    navigateToUserSetting,
    navigateToUserSettingProfile,
    navigateToUserSettingApi,
    closeUserSetting,
    navigateToDataflowResult,
    navigateToDataFile,
    navigateToDatasetSetting,
    navigateToCompilation,
    navigateToDataSourceDetail,
    navigateToMemory,
    navigateToMemorySection,
    navigateToMemoryList,
    navigateToApplicationList,
    navigateToModelSetting,
  };
};
