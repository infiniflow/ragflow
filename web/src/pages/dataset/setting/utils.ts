import { Operator } from '@/constants/agent';
import { DSL, RAGFlowNodeType } from '@/interfaces/database/agent';
import {
  initialExtractorValues,
  initialParserValues,
  initialTitleChunkerValues,
  initialTokenChunkerValues,
  initialTokenizerValues,
} from '@/pages/agent/constant/pipeline';
import {
  transformExtractorParams,
  transformParserParams,
  transformTitleChunkerParams,
  transformTokenChunkerParams,
} from '@/pages/agent/utils';
import { cloneDeep, isEmpty } from 'lodash';

export const FileNodeId = 'File';

export function getOperatorType(operatorId: string): Operator {
  return (operatorId.split(':')[0] || operatorId) as Operator;
}

export function transformParserConfigSetups(
  setups: Record<string, any> | undefined,
): any[] {
  if (!setups || typeof setups !== 'object') {
    return [];
  }

  return Object.entries(setups)
    .map(([fileFormat, config]) => ({
      fileFormat,
      ...config,
    }))
    .sort((a, b) => (a.order_index ?? Infinity) - (b.order_index ?? Infinity));
}

function transformLevelsToRules(
  levels: any[],
): Array<{ levels: Array<{ expression: string }> }> {
  if (!Array.isArray(levels)) return [];
  return levels
    .map((levelGroup) => {
      if (Array.isArray(levelGroup)) {
        const filteredExpressions = levelGroup.filter(
          (expr: string) => expr && expr.trim() !== '',
        );
        if (filteredExpressions.length === 0) return null;
        return {
          levels: filteredExpressions.map((expression: string) => ({
            expression,
          })),
        };
      }
      return { levels: [{ expression: '' }] };
    })
    .filter((rule) => rule !== null);
}

/**
 * Converts Extractor config from API/DSL format to form format.
 * DSL:  { prompts: [{ content: "text", role: "user" }] }
 * Form: { prompts: "text" }
 */
function transformExtractorConfigToForm(
  config: Record<string, any> | undefined,
): Record<string, any> {
  if (!config) return {};

  const result = { ...config };
  if (Array.isArray(config.prompts) && config.prompts.length > 0) {
    result.prompts = config.prompts[0]?.content ?? '';
  }
  return result;
}

/**
 * Converts TokenChunker config from API/DSL format to form format.
 * DSL:  { delimiters: ["\n"], overlapped_percent: 0.1, table_context_size: 10, image_context_size: 20 }
 * Form: { delimiters: [{ value: "\n" }], overlapped_percent: 10, image_table_context_window: 15, enable_children: false }
 */
function transformTokenChunkerConfigToForm(
  config: Record<string, any> | undefined,
): Record<string, any> {
  if (!config) return {};

  const result = { ...config };

  // Convert string array delimiters to object array
  if (Array.isArray(config.delimiters)) {
    result.delimiters = config.delimiters.map((d: string) => ({ value: d }));
  }
  if (Array.isArray(config.children_delimiters)) {
    result.children_delimiters = config.children_delimiters.map(
      (d: string) => ({ value: d }),
    );
  }

  // Convert overlapped_percent from 0-1 scale to 0-100 scale
  if (typeof config.overlapped_percent === 'number') {
    result.overlapped_percent = Math.round(config.overlapped_percent * 100);
  }

  // Merge table_context_size and image_context_size into image_table_context_window
  const tableSize = Number(config.table_context_size ?? 0);
  const imageSize = Number(config.image_context_size ?? 0);
  result.image_table_context_window = Math.max(tableSize, imageSize);

  // Derive delimiter_mode from data
  if (config.delimiter_mode === undefined) {
    const hasDelimiters =
      Array.isArray(config.delimiters) && config.delimiters.length > 0;
    result.delimiter_mode = hasDelimiters ? 'delimiter' : 'token_size';
  }

  // Derive enable_children from presence of children_delimiters
  if (config.enable_children === undefined) {
    result.enable_children =
      Array.isArray(config.children_delimiters) &&
      config.children_delimiters.length > 0;
  }

  // Clean up DSL-only fields not in form schema
  delete result.table_context_size;
  delete result.image_context_size;

  return result;
}

/**
 * Converts TitleChunker config from API/DSL format to form format.
 * DSL:  { method: "hierarchy", hierarchy: "3", levels: [...], include_heading_content, root_chunk_as_heading }
 * Form: { method, hierarchyHierarchy, hierarchyGroup, include_heading_content, root_chunk_as_heading, hierarchyRules, groupRules }
 */
function transformTitleChunkerConfigToForm(
  config: Record<string, any> | undefined,
): Record<string, any> {
  if (!config) return {};

  const result = { ...config };
  const method = config.method ?? 'hierarchy';

  // Convert legacy `hierarchy` (single field) to split fields
  let hierarchy = config.hierarchy;
  if (typeof hierarchy === 'number') {
    hierarchy = String(hierarchy);
  }

  if (method === 'hierarchy') {
    result.hierarchyHierarchy = config.hierarchyHierarchy || hierarchy || '3';
    result.hierarchyGroup = config.hierarchyGroup || '0';
  } else {
    result.hierarchyHierarchy = config.hierarchyHierarchy || hierarchy || '3';
    result.hierarchyGroup = config.hierarchyGroup || hierarchy || '0';
  }

  // Convert `levels` to `hierarchyRules`/`groupRules`
  if (config.levels && Array.isArray(config.levels) && !config.hierarchyRules) {
    result.hierarchyRules = transformLevelsToRules(config.levels);
  }
  if (config.levels && Array.isArray(config.levels) && !config.groupRules) {
    result.groupRules = transformLevelsToRules(config.levels);
  }

  // Clean up DSL-only fields
  delete result.hierarchy;
  delete result.levels;
  delete result.rules;

  return result;
}

/**
 * Tokenizer is a passthrough — both API and form formats are identical.
 */
function transformTokenizerConfigToForm(
  config: Record<string, any> | undefined,
): Record<string, any> {
  return config ?? {};
}

/**
 * Dispatches reverse transform by operator type.
 * Converts DSL-format config to form-format config.
 */
export function transformApiConfigToForm(
  operatorType: string,
  config: Record<string, any> | undefined,
): Record<string, any> {
  switch (operatorType) {
    case Operator.Parser:
      return { setups: transformParserConfigSetups(config) };
    case Operator.Extractor:
      return transformExtractorConfigToForm(config);
    case Operator.Tokenizer:
      return transformTokenizerConfigToForm(config);
    case Operator.TokenChunker:
      return transformTokenChunkerConfigToForm(config);
    case Operator.TitleChunker:
      return transformTitleChunkerConfigToForm(config);
    default:
      return config ?? {};
  }
}

/**
 * Dispatches forward transform by operator type.
 * Converts form-format config to DSL format for saving.
 */
export function transformFormConfigToApi(
  operatorType: string,
  config: Record<string, any> | undefined,
): Record<string, any> {
  if (!config) return {};

  switch (operatorType) {
    case Operator.Parser:
      return transformParserParams(config as any);
    case Operator.Extractor:
      return transformExtractorParams(config as any);
    case Operator.Tokenizer:
      return config; // passthrough for Tokenizer
    case Operator.TokenChunker:
      return transformTokenChunkerParams(config as any);
    case Operator.TitleChunker:
      return transformTitleChunkerParams(config as any);
    default:
      return config;
  }
}

function normalizeOperatorForm(
  operatorId: string,
  rawForm: Record<string, any> | undefined,
): Record<string, any> {
  const operatorType = getOperatorType(operatorId);

  switch (operatorType) {
    case Operator.Parser: {
      return {
        ...cloneDeep(initialParserValues),
        ...rawForm,
        setups: rawForm?.setups?.length
          ? rawForm.setups
          : cloneDeep(initialParserValues.setups),
      };
    }
    case Operator.TitleChunker:
      return {
        ...cloneDeep(initialTitleChunkerValues),
        ...rawForm,
      };
    case Operator.TokenChunker:
      return {
        ...cloneDeep(initialTokenChunkerValues),
        ...rawForm,
      };
    case Operator.Extractor:
      return {
        ...cloneDeep(initialExtractorValues),
        ...rawForm,
      };
    case Operator.Tokenizer:
      return {
        ...cloneDeep(initialTokenizerValues),
        ...rawForm,
      };
    default:
      return rawForm ?? {};
  }
}

export function buildOperatorNode(
  dslNode: RAGFlowNodeType,
  pipelineParserConfig: Record<string, any> = {},
): RAGFlowNodeType {
  const operatorId = dslNode.id;
  const operatorType = getOperatorType(operatorId);

  // Form-format config from DSL graph
  const configFromDsl = dslNode.data?.form;

  const rawForm = {
    ...configFromDsl, // template defaults from DSL (form format)
  };

  if (!isEmpty(pipelineParserConfig)) {
    // user overrides from API (now also form format)
    Object.assign(
      rawForm,
      transformApiConfigToForm(operatorType, pipelineParserConfig[operatorId]), // Convert API config to form format, then merge (DSL template is baseline, API overrides)
    );
  }

  return {
    ...dslNode,
    id: '',
    data: {
      ...dslNode.data,
      label: operatorType,
      operatorId,
      form: normalizeOperatorForm(operatorId, rawForm),
    },
  };
}

export function buildPipelineOperatorNodes(
  dsl?: DSL,
  pipelineParserConfig: Record<string, any> = {},
): RAGFlowNodeType[] {
  if (!dsl?.graph?.nodes || !dsl?.graph?.edges) {
    return [];
  }

  // Build source → target map from edges (pipeline is linear)
  const sourceToTarget = new Map<string, string>();
  for (const edge of dsl.graph.edges) {
    sourceToTarget.set(edge.source, edge.target);
  }

  // Follow the chain starting from File, collecting node IDs in order
  const orderedIds: string[] = [];
  let currentId: string | undefined = FileNodeId;
  while (currentId) {
    orderedIds.push(currentId);
    currentId = sourceToTarget.get(currentId);
  }

  // Build a lookup from node ID → node
  const nodeById = new Map<string, RAGFlowNodeType>();
  for (const node of dsl.graph.nodes) {
    nodeById.set(node.id, node);
  }

  // Map ordered IDs to nodes, excluding File
  return orderedIds
    .filter((id) => id !== FileNodeId)
    .map((id) => nodeById.get(id))
    .filter((node): node is RAGFlowNodeType => node !== undefined)
    .map((node) => buildOperatorNode(node, pipelineParserConfig));
}

/**
 * Builds a parser_config record from operator nodes, keyed by operatorId.
 * Each entry is the node's merged form (DSL template defaults + overrides),
 * i.e. exactly what the operator tabs display.
 */
export function buildParserConfigFromNodes(
  operatorNodes: RAGFlowNodeType[],
): Record<string, any> {
  return Object.fromEntries(
    operatorNodes.map((node) => [
      (node.data as Record<string, any>)?.operatorId || node.data?.label || '',
      node.data?.form ?? {},
    ]),
  );
}
