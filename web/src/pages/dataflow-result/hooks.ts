import message from '@/components/ui/message';
import {
  useCreateChunk,
  useDeleteChunk,
  useSelectChunkList,
} from '@/hooks/chunk-hooks';
import { useSetModalState, useShowDeleteConfirm } from '@/hooks/common-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { IChunk } from '@/interfaces/database/knowledge';
import { buildChunkHighlights } from '@/utils/document-util';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IHighlight } from 'react-pdf-highlighter';
import { ChunkTextMode } from './constant';

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

export const useRerunDataflow = () => {
  const [loading, setLoading] = useState(false);
  return {
    loading,
  };
};

export const useFetchPaserText = () => {
  const initialText =
    '第一行文本\n\t第二行缩进文本\n第三行  多个空格 第一行文本\n\t第二行缩进文本\n第三行 ' +
    '多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格第一行文本\n\t第二行缩进文本\n第三行  ' +
    '多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格第一行文本\n\t第二行缩进文本\n第三行  ' +
    '多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格第一行文本\n\t第二行缩进文本\n第三行  ' +
    '多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格第一行文本\n\t第二行缩进文本\n第三行  ' +
    '多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格第一行文本\n\t第二行缩进文本\n第三行  ' +
    '多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格第一行文本\n\t第二行缩进文本\n第三行  多个空格';
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<string>(initialText);
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const {
    // data,
    // isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createChunk'],
    mutationFn: async (payload: any) => {
      // let service = kbService.create_chunk;
      // if (payload.chunk_id) {
      //   service = kbService.set_chunk;
      // }
      // const { data } = await service(payload);
      // if (data.code === 0) {
      message.success(t('message.created'));
      setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: ['fetchChunkList'] });
      }, 1000); // Delay to ensure the list is updated
      // }
      // return data?.code;
    },
  });

  return { data, loading, rerun: mutateAsync };
};
