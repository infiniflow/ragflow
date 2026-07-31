import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../../store';
import { validateQueritAgentFormValuesForPersistence } from './utils';

export function useQueritAgentFormChange(form?: UseFormReturn<any>) {
  const values = useWatch({ control: form?.control });
  const clickedToolId = useGraphStore((state) => state.clickedToolId);
  const clickedNodeId = useGraphStore((state) => state.clickedNodeId);
  const findUpstreamNodeById = useGraphStore(
    (state) => state.findUpstreamNodeById,
  );
  const updateAgentToolById = useGraphStore(
    (state) => state.updateAgentToolById,
  );

  useEffect(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    if (!agentNode || !form?.formState.isDirty) {
      return;
    }

    const nextValues = validateQueritAgentFormValuesForPersistence(
      form.getValues(),
    );
    if (nextValues) {
      updateAgentToolById(agentNode, clickedToolId, {
        params: nextValues,
      });
    }
  }, [
    clickedNodeId,
    clickedToolId,
    findUpstreamNodeById,
    form,
    form?.formState.isDirty,
    updateAgentToolById,
    values,
  ]);
}
