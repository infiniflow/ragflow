import { Flex } from 'antd';
import classNames from 'classnames';
import get from 'lodash/get';
import { useTranslation } from 'react-i18next';
import { Handle, NodeProps, Position } from 'reactflow';
import {
  BeginQueryType,
  BeginQueryTypeIconMap,
  Operator,
  operatorMap,
} from '../../constant';
import { BeginQuery, NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import { RightHandleStyle } from './handle-icon';
import styles from './index.less';

// TODO: do not allow other nodes to connect to this node
export function BeginNode({ selected, data }: NodeProps<NodeData>) {
  const { t } = useTranslation();
  const query: BeginQuery[] = get(data, 'form.query', []);

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
        style={RightHandleStyle}
      ></Handle>

      <Flex align="center" justify={'center'} gap={10}>
        <OperatorIcon
          name={data.label as Operator}
          fontSize={24}
          color={operatorMap[data.label as Operator].color}
        ></OperatorIcon>
        <div className={styles.nodeTitle}>{t(`flow.begin`)}</div>
      </Flex>
      <Flex gap={8} vertical className={styles.generateParameters}>
        {query.map((x, idx) => {
          const Icon = BeginQueryTypeIconMap[x.type as BeginQueryType];
          return (
            <Flex
              key={idx}
              align="center"
              gap={6}
              className={styles.conditionBlock}
            >
              <Icon className="size-4" />
              <label htmlFor="">{x.key}</label>
              <span className={styles.parameterValue}>{x.name}</span>
              <span className="flex-1">{x.optional ? 'Yes' : 'No'}</span>
            </Flex>
          );
        })}
      </Flex>
    </section>
  );
}
