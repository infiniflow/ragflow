import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';

export function useWatchFormChange(form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });

  const {
    clickedToolId,
    clickedNodeId,
    findUpstreamNodeById,
    getAgentToolById,
    updateAgentToolById,
    updateNodeForm,
  } = useGraphStore();

  useEffect(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    // Manually triggered form updates are synchronized to the canvas
    if (agentNode && form?.formState.isDirty) {
      updateAgentToolById(agentNode, clickedToolId, {
        params: {
          ...(values ?? {}),
        },
      });
    }
  }, [
    clickedNodeId,
    clickedToolId,
    findUpstreamNodeById,
    form,
    form?.formState.isDirty,
    getAgentToolById,
    updateAgentToolById,
    updateNodeForm,
    values,
  ]);
}
