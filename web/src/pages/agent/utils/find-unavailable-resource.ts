import { Operator } from '@/constants/agent';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { IAddedModel } from '@/interfaces/database/llm';
import { getRealModelName, parseModelValue } from '@/utils/llm-util';

const BuiltInParsers = new Set([
  'deepdoc',
  'plain text',
  'docling',
  'opendataloader',
  'tcadp parser',
  'monkeyocrv2',
  'ocr',
]);

function getModelReferences(value: unknown): string[] {
  if (!value || typeof value !== 'object') return [];

  return Object.entries(value).flatMap(([key, child]) => {
    if (
      typeof child === 'string' &&
      child &&
      (['llm_id', 'rerank_id'].includes(key) ||
        (['layout_recognize', 'parse_method'].includes(key) &&
          !BuiltInParsers.has(child.toLowerCase())))
    ) {
      return [child];
    }
    return getModelReferences(child);
  });
}

// Undefined lists mean metadata is not ready, not that every resource is missing.
export function findUnavailableCanvasResource(
  nodes: RAGFlowNodeType[],
  models: IAddedModel[] | undefined,
  groups: ICompilationTemplateGroup[] | undefined,
) {
  for (const node of nodes) {
    for (const reference of getModelReferences(node.data?.form)) {
      if (!models) {
        return { node, messageKey: 'flow.canvasResourcesUnavailable' };
      }
      const parsed = parseModelValue(reference);
      const exists = models.some(
        (model) =>
          model.model_id === reference ||
          (parsed &&
            getRealModelName(model.name) === parsed.model_name &&
            model.instance_name === parsed.model_instance &&
            model.provider_name === parsed.model_provider),
      );
      if (!exists) {
        return { node, messageKey: 'common.modelUnavailable' };
      }
    }

    if (node.data?.label === Operator.Compiler) {
      if (!groups) {
        return { node, messageKey: 'flow.canvasResourcesUnavailable' };
      }
      if (
        !groups.some(
          (group) => group.id === node.data.form?.compilation_template_group_id,
        )
      ) {
        return { node, messageKey: 'flow.compilationOperatorMissing' };
      }
    }
  }
}
