import pick from 'lodash/pick';
import { useCallback, useEffect } from 'react';
import { IOperatorForm } from '../../interface';
import useGraphStore from '../../store';

export const useBuildRelevantOptions = () => {
  const nodes = useGraphStore((state) => state.nodes);

  const buildRelevantOptions = useCallback(
    (toList: string[]) => {
      return nodes
        .filter(
          (x) => !toList.some((y) => y === x.id), // filter out selected values ​​in other to fields from the current drop-down box options
        )
        .map((x) => ({ label: x.data.name, value: x.id }));
    },
    [nodes],
  );

  return buildRelevantOptions;
};

/**
 *  monitor changes in the connection and synchronize the target to the yes and no fields of the form
 *  similar to the categorize-form's useHandleFormValuesChange method
 * @param param0
 */
export const useWatchConnectionChanges = ({ nodeId, form }: IOperatorForm) => {
  const getNode = useGraphStore((state) => state.getNode);
  const node = getNode(nodeId);

  const watchFormChanges = useCallback(() => {
    if (node) {
      form?.setFieldsValue(pick(node, ['yes', 'no']));
    }
  }, [node, form]);

  useEffect(() => {
    watchFormChanges();
  }, [watchFormChanges]);
};
