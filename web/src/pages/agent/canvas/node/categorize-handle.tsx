import { Handle, Position } from '@xyflow/react';

import React from 'react';
import styles from './index.less';

const DEFAULT_HANDLE_STYLE = {
  width: 6,
  height: 6,
  bottom: -5,
  fontSize: 8,
};

interface IProps extends React.PropsWithChildren {
  top: number;
  right: number;
  id: string;
  idx?: number;
}

const CategorizeHandle = ({ top, right, id, children }: IProps) => {
  return (
    <Handle
      type="source"
      position={Position.Right}
      id={id}
      isConnectable
      style={{
        ...DEFAULT_HANDLE_STYLE,
        top: `${top}%`,
        right: `${right}%`,
        background: 'red',
        color: 'black',
      }}
    >
      <span className={styles.categorizeAnchorPointText}>{children || id}</span>
    </Handle>
  );
};

export default CategorizeHandle;
