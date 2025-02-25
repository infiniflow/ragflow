import {
  ICategorizeItem,
  ICategorizeItemResult,
} from '@/interfaces/database/flow';
import omit from 'lodash/omit';
import { useCallback } from 'react';
import { IOperatorForm } from '../../interface';

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
}: IOperatorForm) => {
  const handleValuesChange = useCallback(
    (changedValues: any, values: any) => {
      onValuesChange?.(changedValues, {
        ...omit(values, 'items'),
        category_description: buildCategorizeObjectFromList(values.items),
      });
    },
    [onValuesChange],
  );

  return { handleValuesChange };
};
