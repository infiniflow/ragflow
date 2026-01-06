import { TimelineNode } from '@/components/originui/timeline';
import message from '@/components/ui/message';
import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useFetchMessageTrace } from '@/hooks/use-agent-request';
import { useCreateChunk, useDeleteChunk } from '@/hooks/use-chunk-request';
import kbService from '@/services/knowledge-service';
import { formatSecondsToHumanReadable } from '@/utils/date';
import { buildChunkHighlights } from '@/utils/document-util';
import { useMutation, useQuery } from '@tanstack/react-query';
import { t } from 'i18next';
import { camelCase, upperFirst } from 'lodash';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useParams, useSearchParams } from 'react-router';
import { ITimelineNodeObj, TimelineNodeObj } from './components/time-line';
import {
  ChunkTextMode,
  PipelineResultSearchParams,
  TimelineNodeType,
} from './constant';
import { IChunk, IDslComponent, IPipelineFileLogDetail } from './interface';

export const useFetchPipelineFileLogDetail = ({
  isAgent = false,
  isEdit = true,
  refreshCount,
}: {
  isEdit?: boolean;
  refreshCount?: number;
  isAgent: boolean;
}) => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const logId = searchParams.get('id') || id;

  let queryKey: (string | number)[] = [];
  if (typeof refreshCount === 'number') {
    queryKey = ['fetchLogDetail', refreshCount];
  }

  const { data, isFetching: loading } = useQuery<IPipelineFileLogDetail>({
    queryKey,
    initialData: {} as IPipelineFileLogDetail,
    gcTime: 0,
    enabled: !isAgent,
    queryFn: async () => {
      if (isEdit) {
        const { data } = await kbService.get_pipeline_detail({
          log_id: logId,
        });
        return data?.data ?? {};
      } else {
        return {};
      }
    },
  });

  return { data, loading };
};

export const useHandleChunkCardClick = () => {
  const [selectedChunk, setSelectedChunk] = useState<IChunk>();

  const handleChunkCardClick = useCallback((chunk: IChunk) => {
    setSelectedChunk(chunk);
  }, []);

  return { handleChunkCardClick, selectedChunk };
};

export const useGetChunkHighlights = (selectedChunk?: IChunk) => {
  const [size, setSize] = useState({ width: 849, height: 1200 });

  const highlights: IHighlight[] = useMemo(() => {
    return selectedChunk
      ? buildChunkHighlights(selectedChunk as any, size)
      : [];
  }, [selectedChunk, size]);

  const setWidthAndHeight = useCallback((width: number, height: number) => {
    setSize((pre) => {
      if (pre.height !== height || pre.width !== width) {
        return { height, width };
      }
      return pre;
    });
  }, []);

  return { highlights, setWidthAndHeight };
};

// Switch chunk text to be fully displayed or ellipse
export const useChangeChunkTextMode = () => {
  const [textMode, setTextMode] = useState<ChunkTextMode>(ChunkTextMode.Full);

  const changeChunkTextMode = useCallback((mode: ChunkTextMode) => {
    setTextMode(mode);
  }, []);

  return { textMode, changeChunkTextMode };
};

export const useDeleteChunkByIds = (): {
  removeChunk: (chunkIds: string[], documentId: string) => Promise<number>;
} => {
  const { deleteChunk } = useDeleteChunk();
  const showDeleteConfirm = useShowDeleteConfirm();

  const removeChunk = useCallback(
    (chunkIds: string[], documentId: string) => () => {
      return deleteChunk({ chunkIds, doc_id: documentId });
    },
    [deleteChunk],
  );

  const onRemoveChunk = useCallback(
    (chunkIds: string[], documentId: string): Promise<number> => {
      return showDeleteConfirm({ onOk: removeChunk(chunkIds, documentId) });
    },
    [removeChunk, showDeleteConfirm],
  );

  return {
    removeChunk: onRemoveChunk,
  };
};

export const useUpdateChunk = () => {
  const [chunkId, setChunkId] = useState<string | undefined>('');
  const {
    visible: chunkUpdatingVisible,
    hideModal: hideChunkUpdatingModal,
    showModal,
  } = useSetModalState();
  const { createChunk, loading } = useCreateChunk();
  const { documentId } = useGetKnowledgeSearchParams();

  const onChunkUpdatingOk = useCallback(
    async (params: IChunk) => {
      const code = await createChunk({
        ...params,
        doc_id: documentId,
        chunk_id: chunkId,
      });

      if (code === 0) {
        hideChunkUpdatingModal();
      }
    },
    [createChunk, hideChunkUpdatingModal, chunkId, documentId],
  );

  const handleShowChunkUpdatingModal = useCallback(
    async (id?: string) => {
      setChunkId(id);
      showModal();
    },
    [showModal],
  );

  return {
    chunkUpdatingLoading: loading,
    onChunkUpdatingOk,
    chunkUpdatingVisible,
    hideChunkUpdatingModal,
    showChunkUpdatingModal: handleShowChunkUpdatingModal,
    chunkId,
    documentId,
  };
};

export const useRerunDataflow = ({
  data,
}: {
  data: IPipelineFileLogDetail;
}) => {
  const [isChange, setIsChange] = useState(false);

  const { mutateAsync: handleReRunFunc, isPending: loading } = useMutation({
    mutationKey: ['pipelineRerun', data],
    mutationFn: async (newData: { value: IDslComponent; key: string }) => {
      const newDsl = {
        ...data.dsl,
        components: {
          ...data.dsl.components,
          [newData.key]: newData.value,
        },
      };

      // this Data provided to the interface
      const params = {
        id: data.id,
        dsl: newDsl,
        component_id: newData.key,
      };
      const { data: result } = await kbService.pipelineRerun(params);
      if (result.code === 0) {
        message.success(t('message.operated'));
        // queryClient.invalidateQueries({
        //   queryKey: [type],
        // });
      }
      return result;
    },
  });

  return {
    loading,
    isChange,
    setIsChange,
    handleReRunFunc,
  };
};

export const useTimelineDataFlow = (data: IPipelineFileLogDetail) => {
  const timelineNodes: TimelineNode[] = useMemo(() => {
    const nodes: Array<ITimelineNodeObj & { id: number | string }> = [];
    console.log('time-->', data);
    const times = data?.dsl?.components;
    const graphNodes = data?.dsl?.graph?.nodes;
    if (times) {
      const getNode = (key: string, index: number, type: TimelineNodeType) => {
        const node = times[key].obj;
        const graphNode = graphNodes?.find((item) => item.id === key);
        const name = camelCase(
          node.component_name,
        ) as keyof typeof TimelineNodeObj;

        let tempType = type;
        if (name === TimelineNodeType.parser) {
          tempType = TimelineNodeType.parser;
        } else if (name === TimelineNodeType.tokenizer) {
          tempType = TimelineNodeType.tokenizer;
        } else if (
          name === TimelineNodeType.characterSplitter ||
          name === TimelineNodeType.titleSplitter
        ) {
          tempType = TimelineNodeType.characterSplitter;
        }
        const timeNode = {
          ...TimelineNodeObj[name],
          title: graphNode?.data?.name,
          id: index,
          className: 'w-32',
          completed: false,
          date: formatSecondsToHumanReadable(
            node.params?.outputs?._elapsed_time?.value || 0,
          ),
          type: tempType,
          detail: { value: times[key], key: key },
        } as ITimelineNodeObj & {
          id: number | string;
          className: string;
          completed: boolean;
          date: string;
          type: TimelineNodeType;
          detail: { value: IDslComponent; key: string };
        };
        console.log('timeNodetype-->', type);
        nodes.push(timeNode);

        if (times[key].downstream && times[key].downstream.length > 0) {
          const nextKey = times[key].downstream[0];

          // nodes.push(timeNode);
          getNode(nextKey, index + 1, tempType);
        }
      };
      getNode(upperFirst(TimelineNodeType.begin), 1, TimelineNodeType.begin);
      // setTimelineNodeArr(nodes as unknown as ITimelineNodeObj & {id: number | string})
    }
    return nodes;
  }, [data]);
  return {
    timelineNodes,
  };
};

export const useGetPipelineResultSearchParams = () => {
  const [currentQueryParameters] = useSearchParams();
  const is_read_only = currentQueryParameters.get(
    PipelineResultSearchParams.IsReadOnly,
  ) as 'true' | 'false';
  console.log('is_read_only', is_read_only);
  return {
    type: currentQueryParameters.get(PipelineResultSearchParams.Type) || '',
    documentId:
      currentQueryParameters.get(PipelineResultSearchParams.DocumentId) || '',
    knowledgeId:
      currentQueryParameters.get(PipelineResultSearchParams.KnowledgeId) || '',
    isReadOnly: is_read_only === 'true',
    agentId:
      currentQueryParameters.get(PipelineResultSearchParams.AgentId) || '',
    agentTitle:
      currentQueryParameters.get(PipelineResultSearchParams.AgentTitle) || '',
    documentExtension:
      currentQueryParameters.get(
        PipelineResultSearchParams.DocumentExtension,
      ) || '',
    createdBy:
      currentQueryParameters.get(PipelineResultSearchParams.CreatedBy) || '',
  };
};

export function useFetchPipelineResult({
  agentId,
}: Pick<ReturnType<typeof useGetPipelineResultSearchParams>, 'agentId'>) {
  const [searchParams] = useSearchParams();
  const messageId = searchParams.get('id');

  const { data, setMessageId, setISStopFetchTrace } =
    useFetchMessageTrace(agentId);

  useEffect(() => {
    if (messageId) {
      setMessageId(messageId);
      setISStopFetchTrace(true);
    }
  }, [agentId, messageId, setISStopFetchTrace, setMessageId]);

  const pipelineResult = useMemo(() => {
    if (Array.isArray(data)) {
      const latest = data?.at(-1);
      if (latest?.component_id === 'END' && Array.isArray(latest.trace)) {
        return latest.trace.at(0);
      }
    }
  }, [data]);

  return { pipelineResult };
}

export const useSummaryInfo = (
  data: IPipelineFileLogDetail,
  currentTimeNode: TimelineNode,
) => {
  const summaryInfo = useMemo(() => {
    if (currentTimeNode.type === TimelineNodeType.parser) {
      const setups =
        currentTimeNode.detail?.value?.obj?.params?.setups?.[
          data.document_suffix
        ];
      if (setups) {
        const { output_format, parse_method } = setups;
        const res = [];
        if (parse_method) {
          res.push(`${t('dataflowParser.parserMethod')}: ${parse_method}`);
        }
        if (output_format) {
          res.push(`${t('dataflowParser.outputFormat')}: ${output_format}`);
        }
        return res.join(' ');
      }
    }
    return '';
  }, [data, currentTimeNode]);
  return { summaryInfo };
};
