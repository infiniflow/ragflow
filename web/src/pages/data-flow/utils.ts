import { IAgentForm } from '@/interfaces/database/agent';
import { DSLComponents, RAGFlowNodeType } from '@/interfaces/database/flow';
import { Edge, XYPosition } from '@xyflow/react';
import { FormInstance, FormListFieldData } from 'antd';
import { humanId } from 'human-id';
import { curry, get, intersectionWith, isEmpty, isEqual, sample } from 'lodash';
import pipe from 'lodash/fp/pipe';
import isObject from 'lodash/isObject';
import {
  CategorizeAnchorPointPositions,
  FileType,
  FileTypeSuffixMap,
  NoDebugOperatorsList,
  NodeHandleId,
  Operator,
} from './constant';
import { ExtractorFormSchemaType } from './form/extractor-form';
import { HierarchicalMergerFormSchemaType } from './form/hierarchical-merger-form';
import { ParserFormSchemaType } from './form/parser-form';
import { SplitterFormSchemaType } from './form/splitter-form';
import { IPosition } from './interface';

const buildComponentDownstreamOrUpstream = (
  edges: Edge[],
  nodeId: string,
  isBuildDownstream = true,
) => {
  return edges
    .filter((y) => {
      let isNotUpstreamTool = true;
      let isNotUpstreamAgent = true;
      let isNotExceptionGoto = true;

      return (
        y[isBuildDownstream ? 'source' : 'target'] === nodeId &&
        isNotUpstreamTool &&
        isNotUpstreamAgent &&
        isNotExceptionGoto
      );
    })
    .map((y) => y[isBuildDownstream ? 'target' : 'source']);
};

const removeUselessDataInTheOperator = curry(
  (operatorName: string, params: Record<string, unknown>) => {
    // if (operatorName === Operator.Categorize) {
    //   return removeUselessFieldsFromValues(params, '');
    // }
    return params;
  },
);

const buildOperatorParams = (operatorName: string) =>
  pipe(
    removeUselessDataInTheOperator(operatorName),
    // initializeOperatorParams(operatorName), // Final processing, for guarantee
  );

const ExcludeOperators = [Operator.Note];

export function isBottomSubAgent(edges: Edge[], nodeId?: string) {
  const edge = edges.find(
    (x) => x.target === nodeId && x.targetHandle === NodeHandleId.AgentTop,
  );
  return !!edge;
}
// Because the array of react-hook-form must be object data,
// it needs to be converted into a simple data type array required by the backend
function transformObjectArrayToPureArray(
  list: Array<Record<string, any>>,
  field: string,
) {
  return Array.isArray(list)
    ? list.filter((x) => !isEmpty(x[field])).map((y) => y[field])
    : [];
}

function transformParserParams(params: ParserFormSchemaType) {
  const setups = params.setups.reduce<
    Record<string, ParserFormSchemaType['setups'][0]>
  >((pre, cur) => {
    if (cur.fileFormat) {
      let filteredSetup: Partial<
        ParserFormSchemaType['setups'][0] & { suffix: string[] }
      > = {
        output_format: cur.output_format,
        suffix: FileTypeSuffixMap[cur.fileFormat as FileType],
      };

      switch (cur.fileFormat) {
        case FileType.PDF:
          filteredSetup = {
            ...filteredSetup,
            parse_method: cur.parse_method,
            lang: cur.lang,
          };
          break;
        case FileType.Image:
          filteredSetup = {
            ...filteredSetup,
            parse_method: cur.parse_method,
            lang: cur.lang,
            system_prompt: cur.system_prompt,
          };
          break;
        case FileType.Email:
          filteredSetup = {
            ...filteredSetup,
            fields: cur.fields,
          };
          break;
        case FileType.Video:
        case FileType.Audio:
          filteredSetup = {
            ...filteredSetup,
            llm_id: cur.llm_id,
          };
          break;
        default:
          break;
      }

      pre[cur.fileFormat] = filteredSetup;
    }
    return pre;
  }, {});

  return { ...params, setups };
}

function transformSplitterParams(params: SplitterFormSchemaType) {
  return {
    ...params,
    overlapped_percent: Number(params.overlapped_percent) / 100,
    delimiters: transformObjectArrayToPureArray(params.delimiters, 'value'),
  };
}

function transformHierarchicalMergerParams(
  params: HierarchicalMergerFormSchemaType,
) {
  const levels = params.levels.map((x) =>
    transformObjectArrayToPureArray(x.expressions, 'expression'),
  );

  return { ...params, hierarchy: Number(params.hierarchy), levels };
}

function transformExtractorParams(params: ExtractorFormSchemaType) {
  return { ...params, prompts: [{ content: params.prompts, role: 'user' }] };
}

// construct a dsl based on the node information of the graph
export const buildDslComponentsByGraph = (
  nodes: RAGFlowNodeType[],
  edges: Edge[],
  oldDslComponents: DSLComponents,
): DSLComponents => {
  const components: DSLComponents = {};

  nodes
    ?.filter(
      (x) =>
        !ExcludeOperators.some((y) => y === x.data.label) &&
        !isBottomSubAgent(edges, x.id),
    )
    .forEach((x) => {
      const id = x.id;
      const operatorName = x.data.label;
      let params = x?.data.form ?? {};

      switch (operatorName) {
        case Operator.Parser:
          params = transformParserParams(params);
          break;

        case Operator.Splitter:
          params = transformSplitterParams(params);
          break;

        case Operator.HierarchicalMerger:
          params = transformHierarchicalMergerParams(params);
          break;
        case Operator.Extractor:
          params = transformExtractorParams(params);
          break;

        default:
          break;
      }

      components[id] = {
        obj: {
          ...(oldDslComponents[id]?.obj ?? {}),
          component_name: operatorName,
          params: buildOperatorParams(operatorName)(params) ?? {},
        },
        downstream: buildComponentDownstreamOrUpstream(edges, id, true),
        upstream: buildComponentDownstreamOrUpstream(edges, id, false),
        parent_id: x?.parentId,
      };
    });

  return components;
};

export const receiveMessageError = (res: any) =>
  res && (res?.response.status !== 200 || res?.data?.code !== 0);

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
  currentKeys: string[],
  previousPositionMap: Record<string, IPosition>,
) => {
  // index in use
  const indexesInUse = Object.values(previousPositionMap).map((x) => x.idx);
  const previousKeys = Object.keys(previousPositionMap);
  const intersectionKeys = intersectionWith(
    previousKeys,
    currentKeys,
    (categoryDataKey: string, positionMapKey: string) =>
      categoryDataKey === positionMapKey,
  );
  // difference set
  const currentDifferenceKeys = currentKeys.filter(
    (x) => !intersectionKeys.some((y: string) => y === x),
  );
  const newPositionMap = currentDifferenceKeys.reduce<
    Record<string, IPosition>
  >((pre, cur) => {
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

  return { intersectionKeys, newPositionMap };
};

export const isKeysEqual = (currentKeys: string[], previousKeys: string[]) => {
  return isEqual(currentKeys.sort(), previousKeys.sort());
};

export const getOperatorIndex = (handleTitle: string) => {
  return handleTitle.split(' ').at(-1);
};

// Get the value of other forms except itself
export const getOtherFieldValues = (
  form: FormInstance,
  formListName: string = 'items',
  field: FormListFieldData,
  latestField: string,
) =>
  (form.getFieldValue([formListName]) ?? [])
    .map((x: any) => {
      return get(x, latestField);
    })
    .filter(
      (x: string) =>
        x !== form.getFieldValue([formListName, field.name, latestField]),
    );

export const getNodeDragHandle = (nodeType?: string) => {
  return nodeType === Operator.Note ? '.note-drag-handle' : undefined;
};

const splitName = (name: string) => {
  const names = name.split('_');
  const type = names.at(0);
  const index = Number(names.at(-1));

  return { type, index };
};

export const generateNodeNamesWithIncreasingIndex = (
  name: string,
  nodes: RAGFlowNodeType[],
) => {
  const templateNameList = nodes
    .filter((x) => {
      const temporaryName = x.data.name;

      const { type, index } = splitName(temporaryName);

      return (
        temporaryName.match(/_/g)?.length === 1 &&
        type === name &&
        !isNaN(index)
      );
    })
    .map((x) => {
      const temporaryName = x.data.name;
      const { index } = splitName(temporaryName);

      return {
        idx: index,
        name: temporaryName,
      };
    })
    .sort((a, b) => a.idx - b.idx);

  let index: number = 0;
  for (let i = 0; i < templateNameList.length; i++) {
    const idx = templateNameList[i]?.idx;
    const nextIdx = templateNameList[i + 1]?.idx;
    if (idx + 1 !== nextIdx) {
      index = idx + 1;
      break;
    }
  }

  return `${name}_${index}`;
};

export const duplicateNodeForm = (nodeData?: RAGFlowNodeType['data']) => {
  const form: Record<string, any> = { ...(nodeData?.form ?? {}) };

  return {
    ...(nodeData ?? { label: '' }),
    form,
  };
};

export const getDrawerWidth = () => {
  return window.innerWidth > 1278 ? '40%' : 470;
};

export const needsSingleStepDebugging = (label: string) => {
  return !NoDebugOperatorsList.some((x) => (label as Operator) === x);
};

export const generateDuplicateNode = (
  position?: XYPosition,
  label?: string,
) => {
  const nextPosition = {
    x: (position?.x || 0) + 50,
    y: (position?.y || 0) + 50,
  };

  return {
    selected: false,
    dragging: false,
    id: `${label}:${humanId()}`,
    position: nextPosition,
    dragHandle: getNodeDragHandle(label),
  };
};

export function convertToStringArray(
  list?: Array<{ value: string | number | boolean }>,
) {
  if (!Array.isArray(list)) {
    return [];
  }
  return list.map((x) => x.value);
}

export function convertToObjectArray(list: Array<string | number | boolean>) {
  if (!Array.isArray(list)) {
    return [];
  }
  return list.map((x) => ({ value: x }));
}

export function getAgentNodeTools(agentNode?: RAGFlowNodeType) {
  const tools: IAgentForm['tools'] = get(agentNode, 'data.form.tools', []);
  return tools;
}

export function mapEdgeMouseEvent(
  edges: Edge[],
  edgeId: string,
  isHovered: boolean,
) {
  const nextEdges = edges.map((element) =>
    element.id === edgeId
      ? {
          ...element,
          data: {
            ...element.data,
            isHovered,
          },
        }
      : element,
  );

  return nextEdges;
}
