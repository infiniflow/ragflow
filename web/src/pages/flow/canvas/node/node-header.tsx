import { Flex } from 'antd';

import { Operator, operatorMap } from '../../constant';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import styles from './index.less';

interface IProps {
  id: string;
  label: string;
  name: string;
  gap?: number;
  className?: string;
}

const NodeHeader = ({ label, id, name, gap = 4, className }: IProps) => {
  return (
    <Flex
      flex={1}
      align="center"
      justify={'space-between'}
      gap={gap}
      className={className}
    >
      <OperatorIcon
        name={label as Operator}
        color={operatorMap[label as Operator].color}
      ></OperatorIcon>
      <span className={styles.nodeTitle}>{name}</span>
      <NodeDropdown id={id}></NodeDropdown>
    </Flex>
  );
};

export default NodeHeader;
