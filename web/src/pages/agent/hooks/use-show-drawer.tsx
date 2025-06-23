import { useSetModalState } from '@/hooks/common-hooks';
import { Node, NodeMouseHandler } from '@xyflow/react';
import get from 'lodash/get';
import { useCallback, useEffect } from 'react';
import { Operator } from '../constant';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';
import { useCacheChatLog } from './use-cache-chat-log';
import { useGetBeginNodeDataQuery } from './use-get-begin-query';
import { useSaveGraph } from './use-save-graph';

export const useShowFormDrawer = () => {
  const {
    clickedNodeId: clickNodeId,
    setClickedNodeId,
    getNode,
    setClickedToolId,
  } = useGraphStore((state) => state);
  const {
    visible: formDrawerVisible,
    hideModal: hideFormDrawer,
    showModal: showFormDrawer,
  } = useSetModalState();

  const handleShow: NodeMouseHandler = useCallback(
    (e, node: Node) => {
      setClickedNodeId(node.id);
      setClickedToolId(get(e.target, 'dataset.tool'));
      showFormDrawer();
    },
    [setClickedNodeId, setClickedToolId, showFormDrawer],
  );

  return {
    formDrawerVisible,
    hideFormDrawer,
    showFormDrawer: handleShow,
    clickedNode: getNode(clickNodeId),
  };
};

export const useShowSingleDebugDrawer = () => {
  const { visible, showModal, hideModal } = useSetModalState();
  const { saveGraph } = useSaveGraph();

  const showSingleDebugDrawer = useCallback(async () => {
    const saveRet = await saveGraph();
    if (saveRet?.code === 0) {
      showModal();
    }
  }, [saveGraph, showModal]);

  return {
    singleDebugDrawerVisible: visible,
    hideSingleDebugDrawer: hideModal,
    showSingleDebugDrawer,
  };
};

const ExcludedNodes = [Operator.IterationStart, Operator.Note];

export function useShowDrawer({
  drawerVisible,
  hideDrawer,
}: {
  drawerVisible: boolean;
  hideDrawer(): void;
}) {
  const {
    visible: runVisible,
    showModal: showRunModal,
    hideModal: hideRunModal,
  } = useSetModalState();
  const {
    visible: chatVisible,
    showModal: showChatModal,
    hideModal: hideChatModal,
  } = useSetModalState();
  const {
    singleDebugDrawerVisible,
    showSingleDebugDrawer,
    hideSingleDebugDrawer,
  } = useShowSingleDebugDrawer();
  const { formDrawerVisible, hideFormDrawer, showFormDrawer, clickedNode } =
    useShowFormDrawer();
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();

  useEffect(() => {
    if (drawerVisible) {
      const query: BeginQuery[] = getBeginNodeDataQuery();
      if (query.length > 0) {
        showRunModal();
        hideChatModal();
      } else {
        showChatModal();
        hideRunModal();
      }
    }
  }, [
    hideChatModal,
    hideRunModal,
    showChatModal,
    showRunModal,
    drawerVisible,
    getBeginNodeDataQuery,
  ]);

  const hideRunOrChatDrawer = useCallback(() => {
    hideChatModal();
    hideRunModal();
    hideDrawer();
  }, [hideChatModal, hideDrawer, hideRunModal]);

  const onPaneClick = useCallback(() => {
    hideFormDrawer();
  }, [hideFormDrawer]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (e, node) => {
      if (!ExcludedNodes.some((x) => x === node.data.label)) {
        hideSingleDebugDrawer();
        hideRunOrChatDrawer();
        showFormDrawer(e, node);
      }
      // handle single debug icon click
      if (
        get(e.target, 'dataset.play') === 'true' ||
        get(e.target, 'parentNode.dataset.play') === 'true'
      ) {
        showSingleDebugDrawer();
      }
    },
    [
      hideRunOrChatDrawer,
      hideSingleDebugDrawer,
      showFormDrawer,
      showSingleDebugDrawer,
    ],
  );

  return {
    chatVisible,
    runVisible,
    onPaneClick,
    singleDebugDrawerVisible,
    showSingleDebugDrawer,
    hideSingleDebugDrawer,
    formDrawerVisible,
    showFormDrawer,
    clickedNode,
    onNodeClick,
    hideFormDrawer,
    hideRunOrChatDrawer,
    showChatModal,
  };
}

export function useShowLogSheet({
  setCurrentMessageId,
}: Pick<ReturnType<typeof useCacheChatLog>, 'setCurrentMessageId'>) {
  const { visible, showModal, hideModal } = useSetModalState();

  const handleShow = useCallback(
    (messageId: string) => {
      setCurrentMessageId(messageId);
      showModal();
    },
    [setCurrentMessageId, showModal],
  );

  return {
    logSheetVisible: visible,
    hideLogSheet: hideModal,
    showLogSheet: handleShow,
  };
}
