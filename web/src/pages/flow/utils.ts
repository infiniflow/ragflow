import { DSLComponents } from '@/interfaces/database/flow';
import { removeUselessFieldsFromValues } from '@/utils/form';
import { humanId } from 'human-id';
import { curry, sample } from 'lodash';
import pipe from 'lodash/fp/pipe';
import isObject from 'lodash/isObject';
import { Edge, Node, Position } from 'reactflow';
import { v4 as uuidv4 } from 'uuid';
import { CategorizeAnchorPointPositions, NodeMap, Operator } from './constant';
import { ICategorizeItemResult, IPosition, NodeData } from './interface';

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

// Replace the id in the object with text
export const replaceIdWithText = (
  obj: Record<string, unknown> | unknown[] | unknown,
  getNameById: (id?: string) => string | undefined,
) => {
  if (isObject(obj)) {
    const ret: Record<string, unknown> | unknown[] = Array.isArray(obj)
      ? []
      : {};
    Object.keys(obj).forEach((key) => {
      const val = (obj as Record<string, unknown>)[key];
      const text = typeof val === 'string' ? getNameById(val) : undefined;
      (ret as Record<string, unknown>)[key] = text
        ? text
        : replaceIdWithText(val, getNameById);
    });

    return ret;
  }

  return obj;
};

export const isEdgeEqual = (previous: Edge, current: Edge) =>
  previous.source === current.source &&
  previous.target === current.target &&
  previous.sourceHandle === current.sourceHandle;

export const buildNewPositionMap = (
  categoryDataKeys: string[],
  indexesInUse: number[],
) => {
  return categoryDataKeys.reduce<Record<string, IPosition>>((pre, cur) => {
    // take a coordinate
    const effectiveIdxes = CategorizeAnchorPointPositions.map(
      (x, idx) => idx,
    ).filter((x) => !indexesInUse.some((y) => y === x));
    const idx = sample(effectiveIdxes);
    if (idx !== undefined) {
      indexesInUse.push(idx);
      pre[cur] = { ...CategorizeAnchorPointPositions[idx], idx };
    }

    return pre;
  }, {});
};
