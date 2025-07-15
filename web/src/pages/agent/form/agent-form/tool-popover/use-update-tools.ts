import { IAgentForm } from '@/interfaces/database/agent';
import { DefaultAgentToolValuesMap } from '@/pages/agent/constant';
import { AgentFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { get } from 'lodash';
import { useCallback, useContext, useMemo } from 'react';

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
          pre.push(
            tool
              ? tool
              : {
                  component_name: cur,
                  name: cur,
                  params:
                    DefaultAgentToolValuesMap[
                      cur as keyof typeof DefaultAgentToolValuesMap
                    ] || {},
                },
          );
          return pre;
        }, []);

        updateNodeForm(node?.id, nextValue, ['tools']);
      }
    },
    [node?.id, tools, updateNodeForm],
  );

  return { updateNodeTools };
}

export function useDeleteAgentNodeTools() {
  const { updateNodeForm } = useGraphStore((state) => state);
  const tools = useGetNodeTools();
  const node = useContext(AgentFormContext);

  const deleteNodeTool = useCallback(
    (value: string) => () => {
      const nextTools = tools.filter((x) => x.component_name !== value);
      if (node?.id) {
        updateNodeForm(node?.id, nextTools, ['tools']);
      }
    },
    [node?.id, tools, updateNodeForm],
  );

  return { deleteNodeTool };
}
