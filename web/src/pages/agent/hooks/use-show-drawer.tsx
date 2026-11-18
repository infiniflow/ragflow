import { useSetModalState } from '@/hooks/common-hooks';
import { NodeMouseHandler } from '@xyflow/react';
import get from 'lodash/get';
import React, { useCallback, useEffect } from 'react';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { useCacheChatLog } from './use-cache-chat-log';
import { useGetBeginNodeDataInputs } from './use-get-begin-query';
import { useSaveGraph } from './use-save-graph';

export const useShowFormDrawer = () => {
  const {
    clickedNodeId: clickNodeId,
    setClickedNodeId,
    getNode,
    setClickedToolId,
    getOperatorTypeFromId,
  } = useGraphStore((state) => state);
  const {
    visible: formDrawerVisible,
    hideModal: hideFormDrawer,
    showModal: showFormDrawer,
  } = useSetModalState();

  // Event-free variant for programmatic callers (e.g. the canvas checklist):
  // same guards as handleShow — Tool nodes need a tool id to know which tool
  // form to render, and LoopStart/ExitLoop have no form at all.
  const showFormDrawerById = useCallback(
    (nodeId: string, toolId?: string) => {
      const operatorType = getOperatorTypeFromId(nodeId);
      if (
        (operatorType === Operator.Tool && !toolId) ||
        [Operator.LoopStart, Operator.ExitLoop].includes(
          operatorType as Operator,
        )
      ) {
        return;
      }
      setClickedNodeId(nodeId);
      setClickedToolId(toolId);
      showFormDrawer();
    },
    [getOperatorTypeFromId, setClickedNodeId, setClickedToolId, showFormDrawer],
  );

  const handleShow = useCallback(
    (e: React.MouseEvent<Element>, nodeId: string) => {
      const toolId = (e.target as HTMLElement).dataset.toolId;
      const tool = (e.target as HTMLElement).dataset.tool;

      showFormDrawerById(nodeId, toolId || tool);
    },
    [showFormDrawerById],
  );

  return {
    formDrawerVisible,
    hideFormDrawer,
    showFormDrawer: handleShow,
    showFormDrawerById,
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

const ExcludedNodes = [Operator.Note, Operator.Placeholder, Operator.File];

export function useShowDrawer({
  drawerVisible,
  hideDrawer,
  setCurrentMessageId,
}: {
  drawerVisible: boolean;
  hideDrawer(): void;
} & Pick<ReturnType<typeof useCacheChatLog>, 'setCurrentMessageId'>) {
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
  const {
    formDrawerVisible,
    hideFormDrawer,
    showFormDrawer,
    showFormDrawerById,
    clickedNode,
  } = useShowFormDrawer();
  const inputs = useGetBeginNodeDataInputs();
  const { showLogSheet, logSheetVisible, hideLogSheet } = useShowLogSheet({
    setCurrentMessageId,
  });

  useEffect(() => {
    if (drawerVisible) {
      if (inputs.length > 0) {
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
    inputs,
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
        // hideRunOrChatDrawer();
        hideLogSheet();
        showFormDrawer(e, node.id);
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
      hideLogSheet,
      hideSingleDebugDrawer,
      showFormDrawer,
      showSingleDebugDrawer,
    ],
  );

  const showLogSheetExclusive = useCallback(
    (messageId: string) => {
      hideFormDrawer();
      showLogSheet(messageId);
    },
    [hideFormDrawer, showLogSheet],
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
    showFormDrawerById,
    clickedNode,
    onNodeClick,
    hideFormDrawer,
    hideRunOrChatDrawer,
    showChatModal,
    showLogSheet: showLogSheetExclusive,
    logSheetVisible,
    hideLogSheet,
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

export function useHideFormSheetOnNodeDeletion({
  hideFormDrawer,
}: Pick<ReturnType<typeof useShowFormDrawer>, 'hideFormDrawer'>) {
  const { nodes, clickedNodeId } = useGraphStore((state) => state);

  useEffect(() => {
    if (!nodes.some((x) => x.id === clickedNodeId)) {
      hideFormDrawer();
    }
  }, [clickedNodeId, hideFormDrawer, nodes]);
}
