import {
  AgentStructuredOutputField,
  JsonSchemaDataType,
  Operator,
} from '@/constants/agent';
import { BaseNode } from '@/interfaces/database/agent';
import OperatorIcon from '@/pages/agent/operator-icon';

import { Edge } from '@xyflow/react';
import { get, isEmpty } from 'lodash';
import { ReactNode } from 'react';

export function filterAllUpstreamNodeIds(edges: Edge[], nodeIds: string[]) {
  return nodeIds.reduce<string[]>((pre, nodeId) => {
    const currentEdges = edges.filter((x) => x.target === nodeId);

    const upstreamNodeIds: string[] = currentEdges.map((x) => x.source);

    const ids = upstreamNodeIds.concat(
      filterAllUpstreamNodeIds(edges, upstreamNodeIds),
    );

    ids.forEach((x) => {
      if (pre.every((y) => y !== x)) {
        pre.push(x);
      }
    });

    return pre;
  }, []);
}

export function filterChildNodeIds(nodes: BaseNode[], nodeId?: string) {
  return nodes.filter((x) => x.parentId === nodeId).map((x) => x.id);
}

export function isAgentStructured(id?: string, label?: string) {
  return (
    label === AgentStructuredOutputField && id?.startsWith(`${Operator.Agent}:`)
  );
}

export function buildVariableValue(value: string, nodeId?: string) {
  return `${nodeId}@${value}`;
}

export function buildSecondaryOutputOptions(
  outputs: Record<string, any> = {},
  nodeId?: string,
  parentLabel?: string | ReactNode,
  icon?: ReactNode,
) {
  return Object.keys(outputs).map((x) => ({
    label: x,
    value: buildVariableValue(x, nodeId),
    parentLabel,
    icon,
    type: isAgentStructured(nodeId, x)
      ? JsonSchemaDataType.Object
      : outputs[x]?.type,
  }));
}

export function buildOutputOptions(x: BaseNode) {
  return {
    label: x.data.name,
    value: x.id,
    title: x.data.name,
    options: buildSecondaryOutputOptions(
      x.data.form.outputs,
      x.id,
      x.data.name,
      <OperatorIcon name={x.data.label as Operator} />,
    ),
  };
}

export function buildNodeOutputOptions({
  nodes, // all nodes
  nodeIds, // Need to obtain the output node IDs
}: {
  nodes: BaseNode[];
  nodeIds: string[];
}) {
  const nodeWithOutputList = nodes.filter(
    (x) => nodeIds.some((y) => y === x.id) && !isEmpty(x.data?.form?.outputs),
  );

  return nodeWithOutputList.map((x) => buildOutputOptions(x));
}

export function buildUpstreamNodeOutputOptions({
  nodes,
  edges,
  nodeId,
}: {
  nodes: BaseNode[];
  edges: Edge[];
  nodeId?: string;
}) {
  if (!nodeId) {
    return [];
  }
  const upstreamIds = filterAllUpstreamNodeIds(edges, [nodeId]);

  return buildNodeOutputOptions({ nodes, nodeIds: upstreamIds });
}

export function buildChildOutputOptions({
  nodes,
  nodeId,
}: {
  nodes: BaseNode[];
  nodeId?: string;
}) {
  const nodeWithOutputList = nodes.filter(
    (x) => x.parentId === nodeId && !isEmpty(x.data?.form?.outputs),
  );

  return nodeWithOutputList.map((x) => buildOutputOptions(x));
}

export function getStructuredDatatype(value: Record<string, any> | unknown) {
  const dataType = get(value, 'type');
  const arrayItemsType = get(value, 'items.type', JsonSchemaDataType.String);

  const compositeDataType =
    dataType === JsonSchemaDataType.Array
      ? `${dataType}<${arrayItemsType}>`
      : dataType;

  return { dataType, compositeDataType };
}
