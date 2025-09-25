import { TimelineNode } from '@/components/originui/timeline';
import {
  useCreateChunk,
  useDeleteChunk,
  useSelectChunkList,
} from '@/hooks/chunk-hooks';
import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { IChunk } from '@/interfaces/database/knowledge';
import kbService from '@/services/knowledge-service';
import { formatSecondsToHumanReadable } from '@/utils/date';
import { buildChunkHighlights } from '@/utils/document-util';
import { useQuery } from '@tanstack/react-query';
import { camelCase, upperFirst } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { IHighlight } from 'react-pdf-highlighter';
import { useParams, useSearchParams } from 'umi';
import { ITimelineNodeObj, TimelineNodeObj } from './components/time-line';
import { ChunkTextMode, TimelineNodeType } from './constant';
import { IDslComponent, IPipelineFileLogDetail } from './interface';

export const useFetchPipelineFileLogDetail = (props?: {
  isEdit?: boolean;
  refreshCount?: number;
}) => {
  const { isEdit = true, refreshCount } = props || { isEdit: true };
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
  const [selectedChunkId, setSelectedChunkId] = useState<string>('');

  const handleChunkCardClick = useCallback((chunkId: string) => {
    setSelectedChunkId(chunkId);
  }, []);

  return { handleChunkCardClick, selectedChunkId };
};

export const useGetSelectedChunk = (selectedChunkId: string) => {
  const data = useSelectChunkList();
  return (
    data?.data?.find((x) => x.chunk_id === selectedChunkId) ?? ({} as IChunk)
  );
};

export const useGetChunkHighlights = (selectedChunkId: string) => {
  const [size, setSize] = useState({ width: 849, height: 1200 });
  const selectedChunk: IChunk = useGetSelectedChunk(selectedChunkId);

  const highlights: IHighlight[] = useMemo(() => {
    return buildChunkHighlights(selectedChunk, size);
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

export const useFetchParserList = () => {
  const [loading, setLoading] = useState(false);
  return {
    loading,
  };
};

export const useRerunDataflow = ({
  data,
}: {
  data: IPipelineFileLogDetail;
}) => {
  const [loading, setLoading] = useState(false);
  const [isChange, setIsChange] = useState(false);
  const handleReRunFunc = useCallback(
    (newData: { value: IDslComponent; key: string }) => {
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
        compenent_id: newData.key,
      };
      console.log('newDsl', newDsl, params);
    },
    [data],
  );

  return {
    loading,
    setLoading,
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
    if (times) {
      const getNode = (
        key: string,
        index: number,
        type:
          | TimelineNodeType.begin
          | TimelineNodeType.parser
          | TimelineNodeType.splitter
          | TimelineNodeType.tokenizer
          | TimelineNodeType.characterSplitter
          | TimelineNodeType.titleSplitter,
      ) => {
        const node = times[key].obj;
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
          name === TimelineNodeType.titleSplitter ||
          name === TimelineNodeType.splitter
        ) {
          tempType = TimelineNodeType.splitter;
        }
        const timeNode = {
          ...TimelineNodeObj[name],
          clickable: true,
          id: index,
          className: 'w-32',
          completed: false,
          date: formatSecondsToHumanReadable(
            node.params?.outputs?._elapsed_time?.value || 0,
          ),
          type: tempType,
          detail: { value: times[key], key: key },
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
