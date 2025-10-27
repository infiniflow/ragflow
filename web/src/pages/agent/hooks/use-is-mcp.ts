import { Operator } from '../constant';
import useGraphStore from '../store';

export function useIsMcp(operatorName: Operator) {
  const clickedToolId = useGraphStore((state) => state.clickedToolId);

  return (
    operatorName === Operator.Tool &&
    Object.values(Operator).every((x) => x !== clickedToolId)
  );
}
