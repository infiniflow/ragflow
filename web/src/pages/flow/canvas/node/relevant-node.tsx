import { Handle, NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import classNames from 'classnames';
import { RightHandleStyle } from './handle-icon';

import { useTheme } from '@/components/theme-provider';
import { IRelevantNode } from '@/interfaces/database/flow';
import { get } from 'lodash';
import { useReplaceIdWithName } from '../../hooks';
import styles from './index.less';
import NodeHeader from './node-header';

export function RelevantNode({ id, data, selected }: NodeProps<IRelevantNode>) {
  const yes = get(data, 'form.yes');
  const no = get(data, 'form.no');
  const replaceIdWithName = useReplaceIdWithName();
  const { theme } = useTheme();
  return (
    <section
      className={classNames(
        styles.logicNode,
        theme === 'dark' ? styles.dark : '',
        {
          [styles.selectedNode]: selected,
        },
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable
        className={styles.handle}
        id={'a'}
      ></Handle>
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
        id={'yes'}
        style={{ ...RightHandleStyle, top: 57 + 20 }}
      ></Handle>
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
        id={'no'}
        style={{ ...RightHandleStyle, top: 115 + 20 }}
      ></Handle>
      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        className={styles.nodeHeader}
      ></NodeHeader>

      <Flex vertical gap={10}>
        <Flex vertical>
          <div className={styles.relevantLabel}>Yes</div>
          <div className={styles.nodeText}>{replaceIdWithName(yes)}</div>
        </Flex>
        <Flex vertical>
          <div className={styles.relevantLabel}>No</div>
          <div className={styles.nodeText}>{replaceIdWithName(no)}</div>
        </Flex>
      </Flex>
    </section>
  );
}
