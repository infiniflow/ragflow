import React from 'react';
import { Operator, operatorIconMap } from '../constant';

import styles from './index.less';

interface IProps {
  name: Operator;
  fontSize?: number;
  width?: number;
  color?: string;
}

const OperatorIcon = ({ name, fontSize, width, color }: IProps) => {
  const Icon = operatorIconMap[name] || React.Fragment;
  return (
    <Icon
      className={styles.icon}
      style={{ fontSize, color }}
      width={width}
    ></Icon>
  );
};

export default OperatorIcon;
