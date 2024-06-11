import React from 'react';
import { Operator, operatorIconMap } from '../constant';

import styles from './index.less';

interface IProps {
  name: Operator;
  fontSize?: number;
}

const OperatorIcon = ({ name, fontSize }: IProps) => {
  const Icon = operatorIconMap[name] || React.Fragment;
  return <Icon className={styles.icon} style={{ fontSize }}></Icon>;
};

export default OperatorIcon;
