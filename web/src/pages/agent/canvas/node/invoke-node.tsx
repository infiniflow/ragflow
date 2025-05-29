import { useTheme } from '@/components/theme-provider';
import { IInvokeNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

function InnerInvokeNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IInvokeNode>) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  const url = get(data, 'form.url');
  return (
    <section
      className={classNames(
        styles.ragNode,
        theme === 'dark' ? styles.dark : '',
        {
          [styles.selectedNode]: selected,
        },
      )}
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
  );
}

export const InvokeNode = memo(InnerInvokeNode);
