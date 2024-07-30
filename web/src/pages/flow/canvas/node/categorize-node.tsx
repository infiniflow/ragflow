import { useTranslate } from '@/hooks/common-hooks';
import { Flex } from 'antd';
import classNames from 'classnames';
import { pick } from 'lodash';
import get from 'lodash/get';
import intersectionWith from 'lodash/intersectionWith';
import isEqual from 'lodash/isEqual';
import lowerFirst from 'lodash/lowerFirst';
import { useEffect, useMemo, useState } from 'react';
import { Handle, NodeProps, Position, useUpdateNodeInternals } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { IPosition, NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import { buildNewPositionMap } from '../../utils';
import CategorizeHandle from './categorize-handle';
import NodeDropdown from './dropdown';
import styles from './index.less';
import NodePopover from './popover';

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const updateNodeInternals = useUpdateNodeInternals();
  const [postionMap, setPositionMap] = useState<Record<string, IPosition>>({});
  const categoryData = useMemo(
    () => get(data, 'form.category_description') ?? {},
    [data],
  );
  const style = operatorMap[data.label as Operator];
  const { t } = useTranslate('flow');

  useEffect(() => {
    // Cache used coordinates
    setPositionMap((state) => {
      // index in use
      const indexesInUse = Object.values(state).map((x) => x.idx);
      const categoryDataKeys = Object.keys(categoryData);
      const stateKeys = Object.keys(state);
      if (!isEqual(categoryDataKeys.sort(), stateKeys.sort())) {
        const intersectionKeys = intersectionWith(
          stateKeys,
          categoryDataKeys,
          (categoryDataKey, postionMapKey) => categoryDataKey === postionMapKey,
        );
        const newPositionMap = buildNewPositionMap(
          categoryDataKeys.filter(
            (x) => !intersectionKeys.some((y) => y === x),
          ),
          indexesInUse,
        );

        const nextPostionMap = {
          ...pick(state, intersectionKeys),
          ...newPositionMap,
        };

        return nextPostionMap;
      }
      return state;
    });
  }, [categoryData]);

  useEffect(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals, postionMap]);

  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
          [styles.selectedNode]: selected,
        })}
        style={{
          backgroundColor: style.backgroundColor,
          color: style.color,
        }}
      >
        <Handle
          type="target"
          position={Position.Left}
          isConnectable
          className={styles.handle}
          id={'a'}
        ></Handle>
        <Handle
          type="target"
          position={Position.Top}
          isConnectable
          className={styles.handle}
          id={'b'}
        ></Handle>
        <Handle
          type="target"
          position={Position.Bottom}
          isConnectable
          className={styles.handle}
          id={'c'}
        ></Handle>
        {Object.keys(categoryData).map((x, idx) => {
          const position = postionMap[x];
          return (
            position && (
              <CategorizeHandle
                top={position.top}
                right={position.right}
                key={idx}
                text={x}
                idx={idx}
              ></CategorizeHandle>
            )
          );
        })}
        <Flex vertical align="center" justify="center" gap={6}>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={24}
          ></OperatorIcon>
          <span className={styles.type}>{t(lowerFirst(data.label))}</span>
          <NodeDropdown id={id}></NodeDropdown>
        </Flex>
        <section className={styles.bottomBox}>
          <div className={styles.nodeName}>{data.name}</div>
        </section>
      </section>
    </NodePopover>
  );
}
