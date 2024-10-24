import { Flex } from 'antd';
import classNames from 'classnames';
import { useTranslation } from 'react-i18next';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import { RightHandleStyle } from './handle-icon';
import styles from './index.less';

// TODO: do not allow other nodes to connect to this node
export function BeginNode({ selected, data }: NodeProps<NodeData>) {
  const { t } = useTranslation();

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
      style={{
        width: 100,
      }}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
        style={RightHandleStyle}
      ></Handle>

      <Flex align="center" justify={'space-around'}>
        <OperatorIcon
          name={data.label as Operator}
          fontSize={24}
          color={operatorMap[data.label as Operator].color}
        ></OperatorIcon>
        <div className={styles.nodeTitle}>{t(`flow.begin`)}</div>
      </Flex>
    </section>
  );
}
