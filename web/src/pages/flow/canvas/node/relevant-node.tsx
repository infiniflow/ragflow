import { Flex } from 'antd';
import classNames from 'classnames';
import { PropsWithChildren } from 'react';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import HandleIcon, { RightHandleStyle } from './handle-icon';
import NodePopover from './popover';

import { get } from 'lodash';
import styles from './index.less';

const SourceHandle = ({ children }: PropsWithChildren) => {
  return (
    <Flex align="center" gap={6}>
      <HandleIcon></HandleIcon>
      <span className={styles.relevantSourceLabel}>{children}</span>
    </Flex>
  );
};

export function RelevantNode({ id, data, selected }: NodeProps<NodeData>) {
  const yes = get(data, 'form.yes');
  const no = get(data, 'form.no');
  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
          [styles.selectedNode]: selected,
        })}
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
          style={{ ...RightHandleStyle, top: 60 }}
        ></Handle>
        <Handle
          type="source"
          position={Position.Right}
          isConnectable
          className={styles.handle}
          id={'no'}
          style={{ ...RightHandleStyle, top: 110 }}
        ></Handle>
        <Flex
          align="center"
          justify={'space-between'}
          gap={0}
          flex={1}
          className={styles.nodeHeader}
        >
          <OperatorIcon name={data.label as Operator}></OperatorIcon>
          <span className={styles.nodeTitle}>{data.name}</span>
          <NodeDropdown id={id}></NodeDropdown>
        </Flex>
        <Flex vertical gap={10}>
          <Flex vertical>
            <div>Yes</div>
            <div className={styles.nodeText}>{yes}</div>
          </Flex>
          <Flex vertical>
            <div>No</div>
            <div className={styles.nodeText}>{no}</div>
          </Flex>
        </Flex>
      </section>
    </NodePopover>
  );
}
