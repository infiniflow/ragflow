import get from 'lodash/get';
import omit from 'lodash/omit';
import { useCallback, useEffect, useRef } from 'react';
import { Operator } from '../constant';
import {
  ICategorizeItem,
  ICategorizeItemResult,
  IOperatorForm,
} from '../interface';
import useGraphStore from '../store';

// exclude some nodes downstream of the classification node
const excludedNodes = [Operator.Categorize, Operator.Answer, Operator.Begin];

export const useBuildCategorizeToOptions = () => {
  const nodes = useGraphStore((state) => state.nodes);

  return nodes
    .filter((x) => excludedNodes.every((y) => y !== x.data.label))
    .map((x) => ({ label: x.id, value: x.id }));
};

/**
   * convert the following object into a list
   * 
   * {
      "product_related": {
      "description": "The question is about product usage, appearance and how it works.",
      "examples": "Why it always beaming?\nHow to install it onto the wall?\nIt leaks, what to do?",
      "to": "generate:0"
      }
      }
*/
const buildCategorizeListFromObject = (
  categorizeItem: ICategorizeItemResult,
) => {
  return Object.keys(categorizeItem).reduce<Array<ICategorizeItem>>(
    (pre, cur) => {
      pre.push({ name: cur, ...categorizeItem[cur] });
      return pre;
    },
    [],
  );
};

/**
   * Convert the list in the following form into an object
   * {
    "items": [
      {
        "name": "Categorize 1",
        "description": "111",
        "examples": "ddd",
        "to": "Retrieval:LazyEelsStick"
      }
     ]
    }
*/
const buildCategorizeObjectFromList = (list: Array<ICategorizeItem>) => {
  return list.reduce<ICategorizeItemResult>((pre, cur) => {
    if (cur?.name) {
      pre[cur.name] = omit(cur, 'name');
    }
    return pre;
  }, {});
};

export const useHandleFormValuesChange = ({
  onValuesChange,
  form,
  node,
}: IOperatorForm) => {
  const handleValuesChange = useCallback(
    (changedValues: any, values: any) => {
      console.info(changedValues, values);
      onValuesChange?.(changedValues, {
        ...omit(values, 'items'),
        category_description: buildCategorizeObjectFromList(values.items),
      });
    },
    [onValuesChange],
  );

  useEffect(() => {
    form?.setFieldsValue({
      items: buildCategorizeListFromObject(
        get(node, 'data.form.category_description', {}),
      ),
    });
  }, [form, node]);

  return { handleValuesChange };
};

export const useHandleToSelectChange = (
  opstionIds: string[],
  nodeId?: string,
) => {
  // const [previousTarget, setPreviousTarget] = useState('');
  const previousTarget = useRef('');
  const { addEdge, deleteEdgeBySourceAndTarget } = useGraphStore(
    (state) => state,
  );
  const handleSelectChange = useCallback(
    (value?: string) => {
      if (nodeId) {
        if (previousTarget.current) {
          // delete previous edge
          deleteEdgeBySourceAndTarget(nodeId, previousTarget.current);
        }
        if (value) {
          addEdge({
            source: nodeId,
            target: value,
            sourceHandle: 'b',
            targetHandle: 'd',
          });
        } else {
          // if the value is empty, delete the edges between the current node and all nodes in the drop-down box.
        }
        previousTarget.current = value;
      }
    },
    [addEdge, nodeId, deleteEdgeBySourceAndTarget],
  );

  return { handleSelectChange };
};
