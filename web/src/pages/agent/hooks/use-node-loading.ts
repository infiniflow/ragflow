import {
  INodeData,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { IMessage } from '@/interfaces/database/chat';
import { useCallback, useMemo, useState } from 'react';

export const useNodeLoading = ({
  currentEventListWithoutMessageById,
}: {
  currentEventListWithoutMessageById: (messageId: string) => INodeEvent[];
}) => {
  const [derivedMessages, setDerivedMessages] = useState<IMessage[]>();

  const lastMessageId = useMemo(() => {
    return derivedMessages?.[derivedMessages?.length - 1]?.id;
  }, [derivedMessages]);

  const currentEventListWithoutMessage = useMemo(() => {
    if (!lastMessageId) {
      return [];
    }
    return currentEventListWithoutMessageById(lastMessageId);
  }, [currentEventListWithoutMessageById, lastMessageId]);

  const startedNodeList = useMemo(() => {
    const duplicateList = currentEventListWithoutMessage?.filter(
      (x) => x.event === MessageEventType.NodeStarted,
    ) as INodeEvent[];

    // Remove duplicate nodes
    return duplicateList?.reduce<Array<INodeEvent>>((pre, cur) => {
      if (pre.every((x) => x.data.component_id !== cur.data.component_id)) {
        pre.push(cur);
      }
      return pre;
    }, []);
  }, [currentEventListWithoutMessage]);

  const filterFinishedNodeList = useCallback(() => {
    const nodeEventList = currentEventListWithoutMessage
      .filter(
        (x) => x.event === MessageEventType.NodeFinished,
        // x.event === MessageEventType.NodeFinished &&
        // (x.data as INodeData)?.component_id === componentId,
      )
      .map((x) => x.data);

    return nodeEventList;
  }, [currentEventListWithoutMessage]);

  const lastNode = useMemo(() => {
    if (!startedNodeList) {
      return null;
    }
    return startedNodeList[startedNodeList.length - 1];
  }, [startedNodeList]);

  const startNodeIds = useMemo(() => {
    if (!startedNodeList) {
      return [];
    }
    return startedNodeList.map((x) => x.data.component_id);
  }, [startedNodeList]);

  const finishNodeIds = useMemo(() => {
    if (!lastNode) {
      return [];
    }
    const nodeDataList = filterFinishedNodeList();
    const finishNodeIdsTemp = nodeDataList.map(
      (x: INodeData) => x.component_id,
    );
    return Array.from(new Set(finishNodeIdsTemp));
  }, [lastNode, filterFinishedNodeList]);

  const startButNotFinishedNodeIds = useMemo(() => {
    return startNodeIds.filter((x) => !finishNodeIds.includes(x));
  }, [finishNodeIds, startNodeIds]);

  return {
    lastNode,
    startButNotFinishedNodeIds,
    filterFinishedNodeList,
    setDerivedMessages,
  };
};
