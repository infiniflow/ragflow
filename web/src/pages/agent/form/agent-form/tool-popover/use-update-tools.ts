import { IAgentForm } from '@/interfaces/database/agent';
import { AgentFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { get } from 'lodash';
import { useCallback, useContext } from 'react';

export function useUpdateAgentNodeTools() {
  const { updateNodeForm } = useGraphStore((state) => state);
  const node = useContext(AgentFormContext);

  const updateNodeTools = useCallback(
    (value: string[]) => {
      if (node?.id) {
        const tools: IAgentForm['tools'] = get(node, 'data.form.tools');

        const nextValue = value.reduce<IAgentForm['tools']>((pre, cur) => {
          const tool = tools.find((x) => x.component_name === cur);
          pre.push(tool ? tool : { component_name: cur, params: {} });
          return pre;
        }, []);

        updateNodeForm(node?.id, nextValue, ['tools']);
      }
    },
    [node, updateNodeForm],
  );

  return { updateNodeTools };
}
