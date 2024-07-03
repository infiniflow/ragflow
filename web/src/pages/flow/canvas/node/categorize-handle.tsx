import { Handle, Position } from 'reactflow';
// import { v4 as uuid } from 'uuid';

import styles from './index.less';

const DEFAULT_HANDLE_STYLE = {
  width: 6,
  height: 6,
  bottom: -5,
  fontSize: 8,
};

interface IProps {
  top: number;
  right: number;
  text: string;
  idx?: number;
}

const CategorizeHandle = ({ top, right, text, idx }: IProps) => {
  return (
    <Handle
      type="source"
      position={Position.Right}
      // id={`CategorizeHandle${idx}`}
      id={text}
      isConnectable
      style={{
        ...DEFAULT_HANDLE_STYLE,
        top: `${top}%`,
        right: `${right}%`,
        background: 'red',
        color: 'black',
      }}
    >
      <span className={styles.categorizeAnchorPointText}>{text}</span>
    </Handle>
  );
};

export default CategorizeHandle;
