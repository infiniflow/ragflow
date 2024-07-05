import { DSLComponents } from '@/interfaces/database/flow';
import { removeUselessFieldsFromValues } from '@/utils/form';
import dagre from 'dagre';
import { humanId } from 'human-id';
import { curry } from 'lodash';
import pipe from 'lodash/fp/pipe';
import { Edge, Node, Position } from 'reactflow';
import { v4 as uuidv4 } from 'uuid';
import { NodeMap, Operator } from './constant';
import { ICategorizeItemResult, NodeData } from './interface';

const buildEdges = (
  operatorIds: string[],
  currentId: string,
  allEdges: Edge[],
  isUpstream = false,
  componentName: string,
  nodeParams: Record<string, unknown>,
) => {
  operatorIds.forEach((cur) => {
    const source = isUpstream ? cur : currentId;
    const target = isUpstream ? currentId : cur;
    if (!allEdges.some((e) => e.source === source && e.target === target)) {
      const edge: Edge = {
        id: uuidv4(),
        label: '',
        // type: 'step',
        source: source,
        target: target,
        // markerEnd: {
        //   type: MarkerType.ArrowClosed,
        //   color: 'rgb(157 149 225)',
        //   width: 20,
        //   height: 20,
        // },
      };
      if (componentName === Operator.Categorize && !isUpstream) {
        const categoryDescription =
          nodeParams.category_description as ICategorizeItemResult;

        const name = Object.keys(categoryDescription).find(
          (x) => categoryDescription[x].to === target,
        );

        if (name) {
          edge.sourceHandle = name;
        }
      }
      allEdges.push(edge);
    }
  });
};

export const buildNodesAndEdgesFromDSLComponents = (data: DSLComponents) => {
  const nodes: Node[] = [];
  let edges: Edge[] = [];

  Object.entries(data).forEach(([key, value]) => {
    const downstream = [...value.downstream];
    const upstream = [...value.upstream];
    const { component_name: componentName, params } = value.obj;
    nodes.push({
      id: key,
      type: NodeMap[value.obj.component_name as Operator] || 'ragNode',
      position: { x: 0, y: 0 },
      data: {
        label: componentName,
        name: humanId(),
        form: params,
      },
      sourcePosition: Position.Left,
      targetPosition: Position.Right,
    });

    buildEdges(upstream, key, edges, true, componentName, params);
    buildEdges(downstream, key, edges, false, componentName, params);
  });

  return { nodes, edges };
};

const dagreGraph = new dagre.graphlib.Graph();
dagreGraph.setDefaultEdgeLabel(() => ({}));

const nodeWidth = 172;
const nodeHeight = 36;

export const getLayoutedElements = (
  nodes: Node[],
  edges: Edge[],
  direction = 'TB',
) => {
  const isHorizontal = direction === 'LR';
  dagreGraph.setGraph({ rankdir: direction });

  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, { width: nodeWidth, height: nodeHeight });
  });

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  dagre.layout(dagreGraph);

  nodes.forEach((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    node.targetPosition = isHorizontal ? Position.Left : Position.Top;
    node.sourcePosition = isHorizontal ? Position.Right : Position.Bottom;

    // We are shifting the dagre node position (anchor=center center) to the top left
    // so it matches the React Flow node anchor point (top left).
    node.position = {
      x: nodeWithPosition.x - nodeWidth / 2,
      y: nodeWithPosition.y - nodeHeight / 2,
    };

    return node;
  });

  return { nodes, edges };
};

const buildComponentDownstreamOrUpstream = (
  edges: Edge[],
  nodeId: string,
  isBuildDownstream = true,
) => {
  return edges
    .filter((y) => y[isBuildDownstream ? 'source' : 'target'] === nodeId)
    .map((y) => y[isBuildDownstream ? 'target' : 'source']);
};

const removeUselessDataInTheOperator = curry(
  (operatorName: string, params: Record<string, unknown>) => {
    if (
      operatorName === Operator.Generate ||
      operatorName === Operator.Categorize
    ) {
      return removeUselessFieldsFromValues(params, '');
    }
    return params;
  },
);
// initialize data for operators without parameters
// const initializeOperatorParams = curry((operatorName: string, values: any) => {
//   if (isEmpty(values)) {
//     return initialFormValuesMap[operatorName as Operator];
//   }
//   return values;
// });

const buildOperatorParams = (operatorName: string) =>
  pipe(
    removeUselessDataInTheOperator(operatorName),
    // initializeOperatorParams(operatorName), // Final processing, for guarantee
  );

// construct a dsl based on the node information of the graph
export const buildDslComponentsByGraph = (
  nodes: Node<NodeData>[],
  edges: Edge[],
): DSLComponents => {
  const components: DSLComponents = {};

  nodes.forEach((x) => {
    const id = x.id;
    const operatorName = x.data.label;
    components[id] = {
      obj: {
        component_name: operatorName,
        params:
          buildOperatorParams(operatorName)(
            x.data.form as Record<string, unknown>,
          ) ?? {},
      },
      downstream: buildComponentDownstreamOrUpstream(edges, id, true),
      upstream: buildComponentDownstreamOrUpstream(edges, id, false),
    };
  });

  return components;
};

export const receiveMessageError = (res: any) =>
  res && (res?.response.status !== 200 || res?.data?.retcode !== 0);
