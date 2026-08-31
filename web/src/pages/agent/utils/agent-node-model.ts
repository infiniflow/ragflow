import { Operator } from '@/constants/agent';
import { RAGFlowNodeType } from '@/interfaces/database/agent';

/**
 * An Agent node without a model is unrunnable — the backend only rejects it
 * with `invalid param "model_id": required` once a question is asked. Saving
 * must surface the gap instead of letting it fail at run time.
 */
export const findAgentNodeWithoutModel = (nodes: RAGFlowNodeType[]) =>
  nodes.find(
    (node) => node.data?.label === Operator.Agent && !node.data?.form?.llm_id,
  );
