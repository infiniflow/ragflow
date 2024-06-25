import { Operator } from '../constant';
import useGraphStore from '../store';

// exclude some nodes downstream of the classification node
const excludedNodes = [Operator.Categorize, Operator.Answer, Operator.Begin];

export const useBuildCategorizeToOptions = () => {
  const nodes = useGraphStore((state) => state.nodes);

  return nodes
    .filter((x) => excludedNodes.every((y) => y !== x.data.label))
    .map((x) => ({ label: x.id, value: x.id }));
};
