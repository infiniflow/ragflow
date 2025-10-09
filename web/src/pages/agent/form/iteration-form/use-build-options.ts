import { buildOutputOptions } from '@/utils/canvas-util';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { Operator } from '../../constant';
import useGraphStore from '../../store';

export function useBuildSubNodeOutputOptions(nodeId?: string) {
  const { nodes } = useGraphStore((state) => state);

  const nodeOutputOptions = useMemo(() => {
    if (!nodeId) {
      return [];
    }

    const subNodeWithOutputList = nodes.filter(
      (x) =>
        x.parentId === nodeId &&
        x.data.label !== Operator.IterationStart &&
        !isEmpty(x.data?.form?.outputs),
    );

    return subNodeWithOutputList.map((x) => ({
      label: x.data.name,
      value: x.id,
      title: x.data.name,
      options: buildOutputOptions(x.data.form.outputs, x.id),
    }));
  }, [nodeId, nodes]);

  return nodeOutputOptions;
}
