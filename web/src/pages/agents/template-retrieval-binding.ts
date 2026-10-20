import { DSL } from '@/interfaces/database/agent';
import { Operator, RetrievalFrom } from '@/pages/agent/constant';
import { cloneDeep } from 'lodash';

interface RetrievalParams {
  retrieval_from?: string;
  dataset_ids?: string[];
  memory_ids?: string[];
  kb_ids?: string[];
}

interface RetrievalTool {
  component_name?: string;
  id?: string;
  name?: string;
  params?: RetrievalParams & { tools?: RetrievalTool[] };
}

type AgentParams = RetrievalParams & { tools?: RetrievalTool[] };

interface RetrievalBlockLocation {
  blockId: string;
  displayName: string;
  params: RetrievalParams[];
}

export interface DatasetBlockBinding {
  blockId: string;
  displayName: string;
}

export interface RetrievalBindings {
  datasetBlocks: DatasetBlockBinding[];
  memoryCount: number;
}

function findAgentParams(
  components: Record<string, any>,
  agentId: string,
): AgentParams | undefined {
  const root = components[agentId]?.obj;
  if (root?.component_name === Operator.Agent) return root.params;

  const findNested = (tools: RetrievalTool[] = []): AgentParams | undefined => {
    for (const tool of tools) {
      if (tool.component_name !== Operator.Agent) continue;
      if (tool.id === agentId) return tool.params;
      const nested = findNested(tool.params?.tools);
      if (nested) return nested;
    }
  };

  for (const component of Object.values(components)) {
    const obj = (component as Record<string, any>)?.obj;
    if (obj?.component_name === Operator.Agent) {
      const nested = findNested(obj.params?.tools);
      if (nested) return nested;
    }
  }
}

function collectComponentOnlyBlocks(
  components: Record<string, any>,
): RetrievalBlockLocation[] {
  const blocks: RetrievalBlockLocation[] = [];
  const collectTools = (tools: RetrievalTool[] = [], ownerId: string) => {
    tools.forEach((tool, index) => {
      if (tool.component_name === Operator.Retrieval && tool.params) {
        blocks.push({
          blockId: tool.id || `${ownerId}:tool:${index}`,
          displayName:
            tool.name?.trim() || `${Operator.Retrieval} ${index + 1}`,
          params: [tool.params],
        });
      } else if (tool.component_name === Operator.Agent) {
        collectTools(
          tool.params?.tools,
          tool.id || `${ownerId}:agent:${index}`,
        );
      }
    });
  };

  for (const [componentId, component] of Object.entries(components)) {
    const obj = (component as Record<string, any>)?.obj;
    if (obj?.component_name === Operator.Retrieval && obj.params) {
      blocks.push({
        blockId: componentId,
        displayName: obj.name?.trim() || Operator.Retrieval,
        params: [obj.params],
      });
    } else if (obj?.component_name === Operator.Agent) {
      collectTools(obj.params?.tools, componentId);
    }
  }
  return blocks;
}

function collectRetrievalBlocks(
  dsl: DSL | Record<string, any> | undefined,
): RetrievalBlockLocation[] {
  if (!dsl) return [];

  const dslObject = dsl as Record<string, any>;
  const graphNodes = dslObject.graph?.nodes;
  const components =
    dslObject.components && typeof dslObject.components === 'object'
      ? dslObject.components
      : {};
  if (!Array.isArray(graphNodes)) {
    return collectComponentOnlyBlocks(components);
  }

  const blocks: RetrievalBlockLocation[] = [];
  for (const [nodeIndex, node] of graphNodes.entries()) {
    const form = node?.data?.form;
    if (node?.data?.label === Operator.Retrieval && form) {
      const params = [form as RetrievalParams];
      const component = components[node.id]?.obj;
      if (
        component?.component_name === Operator.Retrieval &&
        component.params
      ) {
        params.push(component.params);
      }
      blocks.push({
        blockId: node.id,
        displayName:
          node.data.name?.trim() || `${Operator.Retrieval} ${nodeIndex + 1}`,
        params,
      });
    } else if (node?.data?.label === Operator.Agent) {
      const componentAgentParams = findAgentParams(components, node.id);
      for (const [toolIndex, tool] of (form?.tools ?? []).entries()) {
        if (
          tool?.component_name !== Operator.Retrieval ||
          !tool.params ||
          typeof tool.params.retrieval_from !== 'string'
        ) {
          continue;
        }
        const params = [tool.params as RetrievalParams];
        const componentTool = componentAgentParams?.tools?.[toolIndex];
        if (
          componentTool?.component_name === Operator.Retrieval &&
          componentTool.params
        ) {
          params.push(componentTool.params);
        }
        blocks.push({
          blockId: tool.id || `${node.id}:tool:${toolIndex}`,
          displayName:
            tool.name?.trim() ||
            node.data.name?.trim() ||
            `${Operator.Retrieval} ${toolIndex + 1}`,
          params,
        });
      }
    }
  }
  return blocks;
}

function isUnbound(
  block: RetrievalBlockLocation,
  field: keyof RetrievalParams,
) {
  return block.params.every((params) => {
    const ids = params[field];
    return !Array.isArray(ids) || ids.length === 0;
  });
}

export function collectUnboundRetrievalBindings(
  dsl: DSL | Record<string, any> | undefined,
): RetrievalBindings {
  const datasetBlocks: DatasetBlockBinding[] = [];
  let memoryCount = 0;

  for (const block of collectRetrievalBlocks(dsl)) {
    const source = block.params[0]?.retrieval_from;
    if (
      source === RetrievalFrom.Dataset &&
      block.params.every((params) => {
        const ids = params.dataset_ids ?? params.kb_ids;
        return !Array.isArray(ids) || ids.length === 0;
      })
    ) {
      datasetBlocks.push({
        blockId: block.blockId,
        displayName: block.displayName,
      });
    } else if (
      source === RetrievalFrom.Memory &&
      isUnbound(block, 'memory_ids')
    ) {
      memoryCount++;
    }
  }
  return { datasetBlocks, memoryCount };
}

export function bindUnboundRetrieval(
  dsl: DSL | Record<string, any> | undefined,
  datasetBindings: Record<string, string[]>,
  memoryIds: string[],
): DSL | undefined {
  if (!dsl) return undefined;

  const next = cloneDeep(dsl) as Record<string, any>;
  for (const block of collectRetrievalBlocks(next)) {
    const source = block.params[0]?.retrieval_from;
    const datasetIds = datasetBindings[block.blockId] ?? [];
    if (
      source === RetrievalFrom.Dataset &&
      datasetIds.length > 0 &&
      block.params.every((params) => {
        const ids = params.dataset_ids ?? params.kb_ids;
        return !Array.isArray(ids) || ids.length === 0;
      })
    ) {
      for (const params of block.params) {
        params.dataset_ids = [...datasetIds];
        delete params.kb_ids;
      }
    } else if (
      source === RetrievalFrom.Memory &&
      memoryIds.length > 0 &&
      isUnbound(block, 'memory_ids')
    ) {
      for (const params of block.params) params.memory_ids = [...memoryIds];
    }
  }
  return next as DSL;
}
