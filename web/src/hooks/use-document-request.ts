import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import { post } from '@/utils/next-request';

import message from '@/components/ui/message';
import { ResponseType } from '@/interfaces/database/base';
import { IReferenceChunk } from '@/interfaces/database/chat';
import {
  IDocumentInfo,
  IDocumentInfoFilter,
} from '@/interfaces/database/document';
import { IChunk } from '@/interfaces/database/knowledge';
import {
  IChangeParserConfigRequestBody,
  IDocumentMetaRequestBody,
} from '@/interfaces/request/document';
import i18n from '@/locales/config';
import { EMPTY_METADATA_FIELD } from '@/pages/dataset/dataset/use-select-filters';
import kbService, { listDocument } from '@/services/knowledge-service';
import api, { api_host } from '@/utils/api';
import { buildChunkHighlights } from '@/utils/document-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { get } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useParams } from 'react-router';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import {
  useGetKnowledgeSearchParams,
  useSetPaginationParams,
} from './route-hook';

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
  WebCrawl = 'webCrawl',
  FetchDocumentThumbnails = 'fetchDocumentThumbnails',
  ParseDocument = 'parseDocument',
}

export const useUploadNextDocument = () => {
  const queryClient = useQueryClient();
  const { id } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<ResponseType<IDocumentInfo[]>, Error, File[]>({
    mutationKey: [DocumentApiAction.UploadDocument],
    mutationFn: async (fileList) => {
      const formData = new FormData();
      formData.append('kb_id', id!);
      fileList.forEach((file: any) => {
        formData.append('file', file);
      });

      try {
        const ret = await kbService.document_upload(formData);
        const code = get(ret, 'data.code');

        if (code === 0 || code === 500) {
          queryClient.invalidateQueries({
            queryKey: [DocumentApiAction.FetchDocumentList],
          });
        }
        return ret?.data;
      } catch (error) {
        console.warn(error);
        return {
          code: 500,
          message: error + '',
        };
      }
    },
  });

  return { uploadDocument: mutateAsync, loading, data };
};

export const useFetchDocumentList = () => {
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
    return docs.some((doc) => doc.run === '1');
  }, [docs]);

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
          kb_id: knowledgeId || id,
          keywords: debouncedSearchString,
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
  const { data } = useQuery({
    queryKey: [
      DocumentApiAction.FetchDocumentFilter,
      debouncedSearchString,
      knowledgeId,
    ],
    queryFn: async () => {
      const { data } = await kbService.documentFilter({
        kb_id: knowledgeId || id,
        keywords: debouncedSearchString,
      });
      if (data.code === 0) {
        return data.data;
      }
    },
  });
  const handleOnpenChange = (e: boolean) => {
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
    onOpenChange: handleOnpenChange,
  };
};
// update document status
export const useSetDocumentStatus = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.UpdateDocumentStatus],
    mutationFn: async ({
      status,
      documentId,
    }: {
      status: boolean;
      documentId: string | string[];
    }) => {
      const ids = Array.isArray(documentId) ? documentId : [documentId];
      const { data } = await kbService.document_change_status({
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
      const ret = await kbService.document_run({
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
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.RemoveDocument],
    mutationFn: async (documentIds: string | string[]) => {
      const { data } = await kbService.document_rm({ doc_id: documentIds });
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
    }: {
      name: string;
      documentId: string;
    }) => {
      const { data } = await kbService.document_rename({
        doc_id: documentId,
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
      parserConfig,
    }: {
      parserId: string;
      pipelineId: string;
      documentId: string;
      parserConfig: IChangeParserConfigRequestBody;
    }) => {
      const { data } = await kbService.document_change_parser({
        parser_id: parserId,
        pipeline_id: pipelineId,
        doc_id: documentId,
        parser_config: parserConfig,
      });
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
        message.error('error');
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
      const { data } = await kbService.document_create({
        name,
        kb_id: id,
      });
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
      return `${api_host}/document/get/${documentId || id}`;
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

export const useNextWebCrawl = () => {
  const { knowledgeId } = useGetKnowledgeSearchParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.WebCrawl],
    mutationFn: async ({ name, url }: { name: string; url: string }) => {
      const formData = new FormData();
      formData.append('name', name);
      formData.append('url', url);
      formData.append('kb_id', knowledgeId);

      const ret = await kbService.web_crawl(formData);
      const code = get(ret, 'data.code');
      if (code === 0) {
        message.success(i18n.t('message.uploaded'));
      }

      return code;
    },
  });

  return {
    data,
    loading,
    webCrawl: mutateAsync,
  };
};

export const useFetchDocumentThumbnailsByIds = () => {
  const [ids, setDocumentIds] = useState<string[]>([]);
  const { data } = useQuery<Record<string, string>>({
    queryKey: [DocumentApiAction.FetchDocumentThumbnails, ids],
    enabled: ids.length > 0,
    initialData: {},
    queryFn: async () => {
      const { data } = await kbService.document_thumbnails({ doc_ids: ids });
      if (data.code === 0) {
        return data.data;
      }
      return {};
    },
  });

  return { data, setDocumentIds };
};

export const useParseDocument = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DocumentApiAction.ParseDocument],
    mutationFn: async (url: string) => {
      try {
        const { data } = await post(api.parse, { url });
        if (data?.code === 0) {
          message.success(i18n.t('message.uploaded'));
        }
        return data;
      } catch (error) {
        message.error('error');
      }
    },
  });

  return { parseDocument: mutateAsync, data, loading };
};
