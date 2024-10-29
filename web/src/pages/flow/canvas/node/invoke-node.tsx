import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { useTranslation } from 'react-i18next';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import NodePopover from './popover';

export function InvokeNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { t } = useTranslation();
  const url = get(data, 'form.url');
  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.ragNode, {
          [styles.selectedNode]: selected,
        })}
      >
        <Handle
          id="c"
          type="source"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={LeftHandleStyle}
        ></Handle>
        <Handle
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          className={styles.handle}
          id="b"
          style={RightHandleStyle}
        ></Handle>
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={styles.nodeHeader}
        ></NodeHeader>
        <Flex vertical>
          <div>{t('flow.url')}</div>
          <div className={styles.nodeText}>{url}</div>
        </Flex>
      </section>
    </NodePopover>
  );
}
