import { IAgentForm } from '@/interfaces/database/agent';
import { AgentFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { get } from 'lodash';
import { useCallback, useContext, useMemo } from 'react';
import { useDeleteToolNode } from '../use-delete-tool-node';

export function useGetNodeTools() {
  const node = useContext(AgentFormContext);

  return useMemo(() => {
    const tools: IAgentForm['tools'] = get(node, 'data.form.tools');
    return tools;
  }, [node]);
}

export function useUpdateAgentNodeTools() {
  const { updateNodeForm } = useGraphStore((state) => state);
  const node = useContext(AgentFormContext);
  const tools = useGetNodeTools();

  const updateNodeTools = useCallback(
    (value: string[]) => {
      if (node?.id) {
        const nextValue = value.reduce<IAgentForm['tools']>((pre, cur) => {
          const tool = tools.find((x) => x.component_name === cur);
          pre.push(tool ? tool : { component_name: cur, params: {} });
          return pre;
        }, []);

        updateNodeForm(node?.id, nextValue, ['tools']);
      }
    },
    [node?.id, tools, updateNodeForm],
  );

  const deleteNodeTool = useCallback(
    (value: string) => {
      updateNodeTools([value]);
    },
    [updateNodeTools],
  );

  return { updateNodeTools, deleteNodeTool };
}

export function useDeleteAgentNodeTools() {
  const { updateNodeForm } = useGraphStore((state) => state);
  const tools = useGetNodeTools();
  const node = useContext(AgentFormContext);
  const { deleteToolNode } = useDeleteToolNode();

  const deleteNodeTool = useCallback(
    (value: string) => () => {
      const nextTools = tools.filter((x) => x.component_name !== value);
      if (node?.id) {
        updateNodeForm(node?.id, nextTools, ['tools']);
        if (nextTools.length === 0) {
          deleteToolNode(node?.id);
        }
      }
    },
    [deleteToolNode, node?.id, tools, updateNodeForm],
  );

  return { deleteNodeTool };
}
