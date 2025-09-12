import message from '@/components/ui/message';
import { trim } from 'lodash';
import {
  ChangeEvent,
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { getAgentNodeTools } from '../utils';

export function useHandleTooNodeNameChange({
  id,
  name,
  setName,
}: {
  id?: string;
  name?: string;
  setName: Dispatch<SetStateAction<string>>;
}) {
  const { clickedToolId, findUpstreamNodeById, updateNodeForm } = useGraphStore(
    (state) => state,
  );
  const agentNode = findUpstreamNodeById(id);
  const tools = getAgentNodeTools(agentNode);

  const previousName = useMemo(() => {
    const tool = tools.find((x) => x.component_name === clickedToolId);
    return tool?.name || tool?.component_name;
  }, [clickedToolId, tools]);

  const handleToolNameBlur = useCallback(() => {
    const trimmedName = trim(name);
    const existsSameName = tools.some((x) => x.name === trimmedName);
    if (trimmedName === '' || existsSameName) {
      if (existsSameName && previousName !== name) {
        message.error('The name cannot be repeated');
      }
      setName(previousName || '');
      return;
    }

    if (agentNode?.id) {
      const nextTools = tools.map((x) => {
        if (x.component_name === clickedToolId) {
          return {
            ...x,
            name,
          };
        }
        return x;
      });
      updateNodeForm(agentNode?.id, nextTools, ['tools']);
    }
  }, [
    agentNode?.id,
    clickedToolId,
    name,
    previousName,
    setName,
    tools,
    updateNodeForm,
  ]);

  return { handleToolNameBlur, previousToolName: previousName };
}

export const useHandleNodeNameChange = ({
  id,
  data,
}: {
  id?: string;
  data: any;
}) => {
  const [name, setName] = useState<string>('');
  const { updateNodeName, nodes, getOperatorTypeFromId } = useGraphStore(
    (state) => state,
  );
  const previousName = data?.name;
  const isToolNode = getOperatorTypeFromId(id) === Operator.Tool;

  const { handleToolNameBlur, previousToolName } = useHandleTooNodeNameChange({
    id,
    name,
    setName,
  });

  const handleNameBlur = useCallback(() => {
    const existsSameName = nodes.some((x) => x.data.name === name);
    if (trim(name) === '' || existsSameName) {
      if (existsSameName && previousName !== name) {
        message.error('The name cannot be repeated');
      }
      setName(previousName);
      return;
    }

    if (id) {
      updateNodeName(id, name);
    }
  }, [name, id, updateNodeName, previousName, nodes]);

  const handleNameChange = useCallback((e: ChangeEvent<any>) => {
    setName(e.target.value);
  }, []);

  useEffect(() => {
    setName(isToolNode ? previousToolName : previousName);
  }, [isToolNode, previousName, previousToolName]);

  return {
    name,
    handleNameBlur: isToolNode ? handleToolNameBlur : handleNameBlur,
    handleNameChange,
  };
};
