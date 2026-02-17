import { Operator } from '../constant';
import useGraphStore from '../store';

export function useIsMcp(operatorName: Operator) {
  const { clickedToolId, getAgentToolById } = useGraphStore();

  const { component_name: toolName } = getAgentToolById(clickedToolId) ?? {};

  return (
    operatorName === Operator.Tool &&
    Object.values(Operator).every((x) => x !== toolName)
  );
}
