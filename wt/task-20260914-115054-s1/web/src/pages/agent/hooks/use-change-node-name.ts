import message from '@/components/ui/message';
import { trim } from 'lodash';
import {
  ChangeEvent,
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { getAgentNodeTools } from '../utils';

export function useHandleToolNodeNameChange({
  id,
  name,
  setName,
}: {
  id?: string;
  name?: string;
  setName: Dispatch<SetStateAction<string>>;
}) {
  const {
    clickedToolId,
    findUpstreamNodeById,
    getAgentToolById,
    updateAgentToolById,
  } = useGraphStore((state) => state);
  const agentNode = findUpstreamNodeById(id)!;
  const tools = getAgentNodeTools(agentNode);
  const previousName = getAgentToolById(clickedToolId, agentNode)?.name;

  const handleToolNameBlur = useCallback(() => {
    const trimmedName = trim(name);
    const existsSameName = tools.some((x) => x.name === trimmedName);

    // Not changed
    if (trimmedName === '') {
      setName(previousName || '');
      return true;
    }

    if (existsSameName && previousName !== name) {
      message.error('The name cannot be repeated');
      return false;
    }

    if (agentNode?.id) {
      updateAgentToolById(agentNode, clickedToolId, { name });
    }

    return true;
  }, [
    agentNode,
    clickedToolId,
    name,
    previousName,
    setName,
    tools,
    updateAgentToolById,
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

  const { handleToolNameBlur, previousToolName } = useHandleToolNodeNameChange({
    id,
    name,
    setName,
  });

  const handleNameBlur = useCallback(() => {
    const trimmedName = trim(name);
    const existsSameName = nodes.some((x) => x.data.name === name);

    // Not changed
    if (!trimmedName) {
      setName(previousName || '');
      return true;
    }

    if (existsSameName && previousName !== name) {
      message.error('The name cannot be repeated');
      return false;
    }

    if (id) {
      updateNodeName(id, name);
    }

    return true;
  }, [name, id, updateNodeName, previousName, nodes]);

  const handleNameChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
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
