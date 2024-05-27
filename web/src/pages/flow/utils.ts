import { DSLComponents } from '@/interfaces/database/flow';
import { Edge, Node, Position } from 'reactflow';
import { v4 as uuidv4 } from 'uuid';

export const buildNodesFromDSLComponents = (data: DSLComponents) => {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  Object.entries(data).forEach(([key, value]) => {
    const downstream = [...value.downstream];
    const upstream = [...value.upstream];
    nodes.push({
      id: key,
      type: 'textUpdater',
      position: { x: 0, y: 0 },
      data: {
        label: value.obj.component_name,
        params: value.obj.params,
        downstream: downstream,
        upstream: upstream,
      },
      sourcePosition: Position.Left,
      targetPosition: Position.Right,
    });

    // intermediate node
    // The first and last nodes do not need to be considered
    if (upstream.length > 0 && downstream.length > 0) {
      for (let i = 0; i < upstream.length; i++) {
        const up = upstream[i];
        for (let j = 0; j < downstream.length; j++) {
          const down = downstream[j];
          edges.push({
            id: uuidv4(),
            label: '',
            type: 'step',
            source: up,
            target: down,
          });
        }
      }
    }
  });
};
