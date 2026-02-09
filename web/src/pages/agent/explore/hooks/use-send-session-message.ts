import sonnerMessage from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useCreateAgentSession,
  useFetchAgent,
} from '@/hooks/use-agent-request';
import { useSendAgentMessage } from '@/pages/agent/chat/use-send-agent-message';
import { buildBeginInputListFromObject } from '@/pages/agent/form/begin-form/utils';
import api from '@/utils/api';
import { get, isEmpty } from 'lodash';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router';
import { BeginId } from '../../constant';
import { useExploreUrlParams } from './use-explore-url-params';

export const useGetBeginNodePrologue = () => {
  const { data } = useFetchAgent();
  const nodes = get(data, 'dsl.graph.nodes', []);

  return useMemo(() => {
    const beginNode = nodes.find((node: any) => node.id === BeginId);
    const formData: Record<string, any> = get(beginNode, 'data.form', {});
    if (formData?.enablePrologue) {
      return formData?.prologue;
    }
  }, [nodes]);
};

export const useSendSessionMessage = () => {
  const { setSessionId, sessionId, isNew } = useExploreUrlParams();

  const { data: canvasInfo } = useFetchAgent();

  const { id: canvasId } = useParams();

  const { createAgentSession } = useCreateAgentSession();

  const isCreatingSession = useRef(false);

  const [beginParams, setBeginParams] = useState<any[]>([]);

  const prologue = useGetBeginNodePrologue();

  const {
    visible: parameterDialogVisible,
    hideModal: hideParameterDialog,
    showModal: showParameterDialog,
  } = useSetModalState();

  const beginInputs = useMemo(() => {
    const beginNode = canvasInfo?.dsl?.graph?.nodes?.find(
      (node: any) => node.id === BeginId,
    );
    const inputs = beginNode?.data?.form?.inputs;
    return buildBeginInputListFromObject(inputs || {});
  }, [canvasInfo]);

  const {
    setDerivedMessages,
    addPrologue,
    derivedMessages,
    handlePressEnter: handleSendPressEnter,
    value,
    ...chatLogic
  } = useSendAgentMessage({
    url: api.runCanvasExplore(canvasId!),
    beginParams,
  });

  const handleParametersOk = useCallback(
    (params: any[]) => {
      setBeginParams(params);
      hideParameterDialog();
    },
    [hideParameterDialog],
  );

  const shouldShowParameterDialog = useCallback(() => {
    if (beginInputs.length > 0 && beginParams.length === 0) {
      showParameterDialog();
    }
  }, [beginInputs, beginParams, showParameterDialog]);

  const handlePressEnter = useCallback(async () => {
    if (isCreatingSession.current) {
      return;
    }

    if (
      prologue &&
      isEmpty(sessionId) &&
      !isNew &&
      derivedMessages.length === 0
    ) {
      addPrologue(prologue);
    }

    let exploreSessionId = sessionId;

    if (isEmpty(sessionId) && canvasId) {
      isCreatingSession.current = true;
      try {
        const sessionName = value?.trim() || 'New Session';
        const result = await createAgentSession({
          id: canvasId,
          name: sessionName,
        });

        exploreSessionId = result.id;

        setSessionId(result.id, false);

        setTimeout(() => {
          isCreatingSession.current = false;
        }, 100);
      } catch (error) {
        isCreatingSession.current = false;
        sonnerMessage.error('Failed to create session');
        console.error('Failed to create session:', error);
        return;
      }
    }

    return handleSendPressEnter?.({ exploreSessionId });
  }, [
    addPrologue,
    canvasId,
    createAgentSession,
    derivedMessages.length,
    handleSendPressEnter,
    isNew,
    prologue,
    sessionId,
    setSessionId,
    value,
  ]);

  useEffect(() => {
    if (isNew && isEmpty(sessionId)) {
      setDerivedMessages([]);
    }
  }, [isNew, sessionId, setDerivedMessages]);

  useEffect(() => {
    if (prologue && isNew && isEmpty(sessionId)) {
      addPrologue(prologue);
    }
  }, [addPrologue, isNew, prologue, sessionId]);

  return {
    ...chatLogic,
    value,
    derivedMessages,
    handlePressEnter,
    canvasInfo,
    parameterDialogVisible,
    handleParametersOk,
    beginInputs,
    shouldShowParameterDialog,
    setDerivedMessages,
  };
};
