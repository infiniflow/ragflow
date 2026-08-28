import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { get, isEmpty } from 'lodash';
import { Operator } from '../constant';
import { FormSchema as ParserFormSchema } from '../form/parser-form';

export type InvalidNodeReason = 'invalidForm' | 'missingModel';

export type InvalidNode = {
  node: RAGFlowNodeType;
  reason: InvalidNodeReason;
};

/**
 * A node's form only validates while its panel is mounted, so bad values can
 * sit in the store long after the user has moved on. Re-check them against the
 * same schema before persisting, but only for nodes the user actually edited —
 * a node still carrying stale DSL from an older version must not wedge saving.
 */
export function findInvalidNode(
  nodes: RAGFlowNodeType[],
  editedNodeIds: string[],
): InvalidNode | undefined {
  const invalidFormNode = nodes.find(
    (node) =>
      editedNodeIds.includes(node.id) &&
      node.data?.label === Operator.Parser &&
      !ParserFormSchema.safeParse(node.data?.form).success,
  );
  if (invalidFormNode) {
    return { node: invalidFormNode, reason: 'invalidForm' };
  }
  // An Agent without a model always fails at run time with
  // `invalid param "model_id": required`, so surface it when saving instead.
  const missingModelNode = nodes.find(
    (node) =>
      editedNodeIds.includes(node.id) &&
      node.data?.label === Operator.Agent &&
      isEmpty(get(node.data, 'form.llm_id')),
  );
  if (missingModelNode) {
    return { node: missingModelNode, reason: 'missingModel' };
  }
  return undefined;
}