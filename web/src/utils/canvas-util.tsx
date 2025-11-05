import { BaseNode } from '@/interfaces/database/agent';
import { Edge } from '@xyflow/react';
import { isEmpty } from 'lodash';
import { ComponentType, ReactNode } from 'react';

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

export function buildOutputOptions(
  outputs: Record<string, any> = {},
  nodeId?: string,
  parentLabel?: string | ReactNode,
  icon?: ReactNode,
) {
  return Object.keys(outputs).map((x) => ({
    label: x,
    value: `${nodeId}@${x}`,
    parentLabel,
    icon,
    type: outputs[x]?.type,
  }));
}

export function buildNodeOutputOptions({
  nodes,
  edges,
  nodeId,
  Icon,
}: {
  nodes: BaseNode[];
  edges: Edge[];
  nodeId?: string;
  Icon: ComponentType<{ name: string }>;
}) {
  if (!nodeId) {
    return [];
  }
  const upstreamIds = filterAllUpstreamNodeIds(edges, [nodeId]);

  const nodeWithOutputList = nodes.filter(
    (x) =>
      upstreamIds.some((y) => y === x.id) && !isEmpty(x.data?.form?.outputs),
  );

  return nodeWithOutputList
    .filter((x) => x.id !== nodeId)
    .map((x) => ({
      label: x.data.name,
      value: x.id,
      title: x.data.name,
      options: buildOutputOptions(
        x.data.form.outputs,
        x.id,
        x.data.name,
        <Icon name={x.data.name} />,
      ),
    }));
}
