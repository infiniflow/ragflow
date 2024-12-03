import get from 'lodash/get';
import omit from 'lodash/omit';
import { useCallback, useEffect } from 'react';
import {
  ICategorizeItem,
  ICategorizeItemResult,
  IOperatorForm,
} from '../../interface';
import useGraphStore from '../../store';

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
  // Categorize's to field has two data sources, with edges as the data source.
  // Changes in the edge or to field need to be synchronized to the form field.
  return Object.keys(categorizeItem)
    .reduce<Array<ICategorizeItem>>((pre, cur) => {
      // synchronize edge data to the to field

      pre.push({ name: cur, ...categorizeItem[cur] });
      return pre;
    }, [])
    .sort((a, b) => a.index - b.index);
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
  nodeId,
}: IOperatorForm) => {
  const getNode = useGraphStore((state) => state.getNode);
  const node = getNode(nodeId);

  const handleValuesChange = useCallback(
    (changedValues: any, values: any) => {
      onValuesChange?.(changedValues, {
        ...omit(values, 'items'),
        category_description: buildCategorizeObjectFromList(values.items),
      });
    },
    [onValuesChange],
  );

  useEffect(() => {
    const items = buildCategorizeListFromObject(
      get(node, 'data.form.category_description', {}),
    );
    form?.setFieldsValue({
      items,
    });
  }, [form, node]);

  return { handleValuesChange };
};
