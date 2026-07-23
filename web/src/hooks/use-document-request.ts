import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';

import message from '@/components/ui/message';
import { RunningStatus } from '@/constants/knowledge';
import { ResponseType } from '@/interfaces/database/base';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/dataset';
import {
  IDocumentInfo,
  IDocumentInfoFilter,
} from '@/interfaces/database/document';
import { IStructureGraphResponse } from '@/interfaces/database/document-structure';
import {
  IChangeParserConfigRequestBody,
  IDocumentMetaRequestBody,
} from '@/interfaces/request/document';
import i18n from '@/locales/config';
import { EMPTY_METADATA_FIELD } from '@/pages/dataset/dataset/use-select-filters';
import documentStructureService from '@/services/document-structure-service';
import kbService, {
  changeDocumentParser,
  changeDocumentsStatus,
  createDocument,
  deleteDocument,
  documentFilter,
  listDocument,
  renameDocument,
  uploadDocument,
} from '@/services/knowledge-service';
import { restAPIv1 } from '@/utils/api';
import { buildChunkHighlights } from '@/utils/document-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { get } from 'lodash';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useParams } from 'react-router';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import {
  extractParserConfigExt,
  isPipelineParserConfig,
} from './parser-config-utils';
import {
  useGetKnowledgeSearchParams,
  useSetPaginationParams,
} from './route-hook';
import { KnowledgeApiAction } from './use-knowledge-request';

export const enum DocumentApiAction {
  UploadDocument = 'uploadDocument',
  FetchDocumentList = 'fetchDocumentList',
  UpdateDocumentStatus = 'updateDocumentStatus',
  RunDocumentByIds = 'runDocumentByIds',
  RemoveDocument = 'removeDocument',
  SaveDocumentName = 'saveDocumentName',
  SetDocumentParser = 'setDocumentParser',
  SetDocumentMeta = 'setDocumentMeta',
  FetchDocumentFilter = 'fetchDocumentFilter',
  CreateDocument = 'createDocument',
  FetchDocumentThumbnails = 'fetchDocumentThumbnails',
  ParseDocument = 'parseDocument',
}

export const enum DocumentStructureApiAction {
  FetchDocumentStructureGraph = 'fetchDocumentStructureGraph',
  DeleteDocumentStructureGraph = 'deleteDocumentStructureGraph',
}

const DocumentKeys = {
  byIds: (ids: string[]) =>
    [DocumentApiAction.FetchDocumentList, 'byIds', ids] as const,
};

export const DocumentStructureKeys = {
  graph: (datasetId: string, documentId: string) =>
    [
      DocumentStructureApiAction.FetchDocumentStructureGraph,
      datasetId,
      documentId,
    ] as const,
};

export const useUploadDocument = () => {
  const queryClient = useQueryClient();
  const { id } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<
    ResponseType<IDocumentInfo[]>,
    Error,
    { fileList: File[]; parserConfig?: Record<string, any> }
  >({
    mutationKey: [DocumentApiAction.UploadDocument],
    mutationFn: async ({ fileList, parserConfig }) => {
      if (!id) {
        return { code: 500, message: 'Dataset ID is required' };
      }
      const formData = new FormData();
      fileList.forEach((file: any) => {
        formData.append('file', file);
      });
      if (parserConfig) {
        formData.append('parser_config', JSON.stringify(parserConfig));
      }

      try {
        const ret = await uploadDocument(id, formData);
        const code = get(ret, 'code');

        if (code === 0 || code === 500) {
          queryClient.invalidateQueries({
            queryKey: [DocumentApiAction.FetchDocumentList],
          });
        }
        return ret;
      } catch (error) {
        console.warn(error);
        return {
          code: 500,
          message: error + '',
        };
      }
    },
  });

  const upload = useCallback(
    (fileList: File[], parserConfig?: Record<string, any>) =>
      mutateAsync({ fileList, parserConfig }),
    [mutateAsync],
  );

  return { uploadDocument: upload, loading, data };
};

export const useFetchDocumentList = (loop = true) => {
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const { filterValue, handleFilterSubmit, checkValue } =
    useHandleFilterSubmit();
  const [docs, setDocs] = useState<IDocumentInfo[]>([]);

  const isLoop = useMemo(() => {
    return loop && docs.some((doc) => doc.run === RunningStatus.RUNNING);
  }, [docs, loop]);

  const { data, isFetching: loading } = useQuery<{
    docs: IDocumentInfo[];
    total: number;
  }>({
    queryKey: [
      DocumentApiAction.FetchDocumentList,
      debouncedSearchString,
      pagination,
      filterValue,
    ],
    initialData: { docs: [], total: 0 },
    refetchInterval: isLoop ? 5000 : false,
    enabled: !!knowledgeId || !!id,
    queryFn: async () => {
      let run = [] as any;
      let returnEmptyMetadata = false;
      if (filterValue.run && Array.isArray(filterValue.run)) {
        run = [...(filterValue.run as string[])];
        const returnEmptyMetadataIndex = run.findIndex(
          (r: string) => r === EMPTY_METADATA_FIELD,
        );
        if (returnEmptyMetadataIndex > -1) {
          returnEmptyMetadata = true;
          run.splice(returnEmptyMetadataIndex, 1);
        }
      } else {
        run = filterValue.run;
      }
      const ret = await listDocument(
        {
          id: knowledgeId || id,
          ext: { keywords: debouncedSearchString },
          page_size: pagination.pageSize,
          page: pagination.current,
        },
        {
          suffix: filterValue.type as string[],
          run_status: run as string[],
          return_empty_metadata: returnEmptyMetadata,
          metadata: filterValue.metadata as Record<string, string[]>,
        },
      );
      if (ret.data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentFilter],
        });
        return ret.data.data;
      }

      return {
        docs: [],
        total: 0,
      };
    },
  });
  useMemo(() => {
    setDocs(data.docs);
  }, [data.docs]);
  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      setPagination({ page: 1 });
      handleInputChange(e);
    },
    [handleInputChange, setPagination],
  );

  useEffect(() => {
    queryClient.invalidateQueries({
      queryKey: [KnowledgeApiAction.FetchKnowledgeDetail],
    });
  }, [data.docs, queryClient]);

  return {
    loading,
    searchString,
    documents: data.docs,
    pagination: { ...pagination, total: data?.total },
    handleInputChange: onInputChange,
    setPagination,
    filterValue,
    handleFilterSubmit,
    checkValue,
  };
};

export const useFetchDocumentsByIds = (ids: string[]) => {
  const { id: datasetId } = useParams();

  const { data, isFetching: loading } = useQuery<{
    docs: IDocumentInfo[];
    total: number;
  }>({
    queryKey: DocumentKeys.byIds(ids),
    enabled: ids.length > 0 && !!datasetId,
    initialData: { docs: [], total: 0 },
    queryFn: async () => {
      const ret = await listDocument(
        {
          id: datasetId,
          page: 1,
          page_size: ids.length,
        },
        {
          ids,
        },
      );
      if (ret.data.code === 0) {
        return ret.data.data;
      }
      return { docs: [], total: 0 };
    },
  });

  return { documents: data.docs, loading };
};

// get document filter
export const useGetDocumentFilter = (): {
  filter: IDocumentInfoFilter;
  onOpenChange: (open: boolean) => void;
} => {
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const { searchString } = useHandleSearchChange();
  const { id } = useParams();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const [open, setOpen] = useState<number>(0);
  const datasetId = knowledgeId || id;
  const { data } = useQuery({
    queryKey: [
      DocumentApiAction.FetchDocumentFilter,
      debouncedSearchString,
      knowledgeId,
    ],
    queryFn: async () => {
      if (!datasetId) {
        return;
      }
      const { data } = await documentFilter(datasetId);
      if (data.code === 0) {
        return data.data;
      }
    },
  });
  const handleOpenChange = (e: boolean) => {
    if (e) {
      const currentOpen = open + 1;
      setOpen(currentOpen);
    }
  };
  return {
    filter: data?.filter || {
      run_status: {},
      suffix: {},
      metadata: {},
    },
    onOpenChange: handleOpenChange,
  };
};
// update document status
export const useSetDocumentStatus = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<
    any,
    Error,
    {
      status: boolean;
      documentId: string | string[];
      datasetId: string;
    }
  >({
    mutationKey: [DocumentApiAction.UpdateDocumentStatus],
    mutationFn: async ({ status, documentId, datasetId }) => {
      const ids = Array.isArray(documentId) ? documentId : [documentId];
      const { data } = await changeDocumentsStatus({
        kb_id: datasetId,
        doc_ids: ids,
        status: Number(status),
      });

      if (data.code === 0) {
        message.success(i18n.t('message.modified'));
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
      }
      return data;
    },
  });

  return { setDocumentStatus: mutateAsync, data, loading };
};

// This hook is used to run a document by its IDs
export const useRunDocument = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.RunDocumentByIds],
    mutationFn: async ({
      documentIds,
      run,
      option,
    }: {
      documentIds: string[];
      run: number;
      option?: { delete: boolean; apply_kb: boolean };
    }) => {
      queryClient.invalidateQueries({
        queryKey: [DocumentApiAction.FetchDocumentList],
      });
      const ret = await kbService.documentIngest({
        doc_ids: documentIds,
        run,
        ...(option || {}),
      });
      const code = get(ret, 'data.code');
      if (code === 0) {
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
        message.success(i18n.t('message.operated'));
      }

      return code;
    },
  });

  return { runDocumentByIds: mutateAsync, loading, data };
};

export const useRemoveDocument = () => {
  const queryClient = useQueryClient();
  const { id: datasetId } = useParams();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.RemoveDocument],
    mutationFn: async (documentIds: string | string[]) => {
      const ids = Array.isArray(documentIds) ? documentIds : [documentIds];
      const { data } = await deleteDocument(datasetId!, ids);
      if (data.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
      }
      return data.code;
    },
  });

  return { data, loading, removeDocument: mutateAsync };
};

export const useSaveDocumentName = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.SaveDocumentName],
    mutationFn: async ({
      name,
      documentId,
      kbId,
    }: {
      name: string;
      documentId: string;
      kbId: string;
    }) => {
      const { data } = await renameDocument(kbId, documentId, {
        name: name,
      });
      if (data.code === 0) {
        message.success(i18n.t('message.renamed'));
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
      }
      return data.code;
    },
  });

  return { loading, saveName: mutateAsync, data };
};

export const useSetDocumentParser = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.SetDocumentParser],
    mutationFn: async ({
      parserId,
      pipelineId,
      documentId,
      datasetId,
      parserConfig,
    }: {
      parserId: string;
      pipelineId: string;
      documentId: string;
      datasetId: string;
      parserConfig?: IChangeParserConfigRequestBody;
    }) => {
      // Build update payload
      const updateData: Record<string, unknown> = {};
      if (parserId) {
        updateData.chunk_method = parserId;
      }
      if (pipelineId) {
        updateData.pipeline_id = pipelineId;
      }

      if (parserConfig) {
        updateData.parser_config = extractParserConfigExt(parserConfig);
      }

      const { data } = await changeDocumentParser(
        datasetId,
        documentId,
        updateData,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });

        message.success(i18n.t('message.modified'));
      }
      return data.code;
    },
  });

  return { setDocumentParser: mutateAsync, data, loading };
};

/**
 * Go-backend variant of useSetDocumentParser. The Go document endpoint takes
 * `parser_id` (instead of the legacy `chunk_method`) and expects the
 * pipeline-shaped parser_config (keyed by operator id) to be sent as-is.
 * Keep it parallel to the Python version — the original hook stays untouched
 * and can be dropped once the Python backend is retired.
 */
export const useSetDocumentPipelineParser = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.SetDocumentParser, 'pipeline'],
    mutationFn: async ({
      parserId,
      pipelineId,
      parseType,
      documentId,
      datasetId,
      parserConfig,
    }: {
      parserId: string;
      pipelineId: string;
      parseType?: number;
      documentId: string;
      datasetId: string;
      parserConfig?: IChangeParserConfigRequestBody;
    }) => {
      const updateData: Record<string, unknown> = {
        parser_id: parserId,
        pipeline_id: pipelineId,
      };

      if (parseType !== undefined) {
        updateData.parse_type = parseType;
      }

      if (parserConfig) {
        updateData.parser_config = isPipelineParserConfig(parserConfig)
          ? parserConfig
          : extractParserConfigExt(parserConfig);
      }

      const { data } = await changeDocumentParser(
        datasetId,
        documentId,
        updateData,
      );
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });

        message.success(i18n.t('message.modified'));
      }
      return data.code;
    },
  });

  return { setDocumentPipelineParser: mutateAsync, data, loading };
};

export const useSetDocumentMeta = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.SetDocumentMeta],
    mutationFn: async (params: IDocumentMetaRequestBody) => {
      try {
        const { data } = await kbService.setMeta({
          meta: params.meta,
          doc_id: params.documentId,
        });

        if (data?.code === 0) {
          queryClient.invalidateQueries({
            queryKey: [DocumentApiAction.FetchDocumentList],
          });

          message.success(i18n.t('message.modified'));
        }
        return data?.code;
      } catch (error) {
        message.error('error:' + error);
      }
    },
  });

  return { setDocumentMeta: mutateAsync, data, loading };
};

export const useCreateDocument = () => {
  const { id } = useParams();
  const { setPaginationParams, page } = useSetPaginationParams();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.CreateDocument],
    mutationFn: async (name: string) => {
      if (!id) {
        return 500;
      }
      const data = await createDocument(id, name);
      if (data.code === 0) {
        if (page === 1) {
          queryClient.invalidateQueries({
            queryKey: [DocumentApiAction.FetchDocumentList],
          });
        } else {
          setPaginationParams(); // fetch document list
        }

        message.success(i18n.t('message.created'));
      }
      return data.code;
    },
  });

  return { createDocument: mutateAsync, loading, data };
};

export const useGetDocumentUrl = (documentId?: string) => {
  const getDocumentUrl = useCallback(
    (id?: string) => {
      return `${restAPIv1}/documents/${id || documentId}/preview`;
    },
    [documentId],
  );

  return getDocumentUrl;
};

export const useGetChunkHighlights = (
  selectedChunk: IChunk | IReferenceChunk,
) => {
  const [size, setSize] = useState({ width: 849, height: 1200 });

  const highlights: IHighlight[] = useMemo(() => {
    return buildChunkHighlights(selectedChunk, size);
  }, [selectedChunk, size]);

  const setWidthAndHeight = (width: number, height: number) => {
    setSize((pre) => {
      if (pre.height !== height || pre.width !== width) {
        return { height, width };
      }
      return pre;
    });
  };

  return { highlights, setWidthAndHeight };
};

export const useFetchDocumentThumbnailsByIds = () => {
  const [ids, setDocumentIds] = useState<string[]>([]);
  const { data } = useQuery<Record<string, string>>({
    queryKey: [DocumentApiAction.FetchDocumentThumbnails, ids],
    enabled: ids.length > 0,
    initialData: {},
    queryFn: async () => {
      const { data } = await kbService.documentThumbnails({ doc_ids: ids });
      if (data.code === 0) {
        return data.data;
      }
      return {};
    },
  });

  return { data, setDocumentIds };
};

export function useFetchDocumentStructureGraph() {
  const { knowledgeId: datasetId, documentId } = useGetKnowledgeSearchParams();
  const enabled = !!datasetId && !!documentId;

  const { data, isFetching: loading } =
    useQuery<IStructureGraphResponse | null>({
      queryKey: DocumentStructureKeys.graph(datasetId, documentId),
      enabled,
      initialData: null,
      gcTime: 0,
      queryFn: async () => {
        const { data } =
          await documentStructureService.getDocumentStructureGraph(
            datasetId,
            documentId,
          );
        return data?.data ?? null;
      },
    });

  return { data, loading };
}

export function useDeleteDocumentStructureGraph() {
  const { knowledgeId: datasetId, documentId } = useGetKnowledgeSearchParams();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentStructureApiAction.DeleteDocumentStructureGraph],
    mutationFn: async (templateId: string) => {
      const { data } =
        await documentStructureService.deleteDocumentStructureGraph(
          datasetId,
          documentId,
          templateId,
        );
      if (data.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: DocumentStructureKeys.graph(datasetId, documentId),
        });
      }
      return data;
    },
  });

  return { deleteDocumentStructureGraph: mutateAsync, loading, data };
}
