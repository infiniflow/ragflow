import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { Handle, NodeProps, Position } from 'reactflow';
import { useGetComponentLabelByValue } from '../../hooks';
import { IGenerateParameter, NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';

import styles from './index.less';

export function TemplateNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const parameters: IGenerateParameter[] = get(data, 'form.parameters', []);
  const getLabel = useGetComponentLabelByValue(id);

  return (
    <section
      className={classNames(styles.logicNode, {
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
        style={RightHandleStyle}
        id="b"
      ></Handle>

      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        className={styles.nodeHeader}
      ></NodeHeader>

      <Flex gap={8} vertical className={styles.generateParameters}>
        {parameters.map((x) => (
          <Flex
            key={x.id}
            align="center"
            gap={6}
            className={styles.conditionBlock}
          >
            <label htmlFor="">{x.key}</label>
            <span className={styles.parameterValue}>
              {getLabel(x.component_id)}
            </span>
          </Flex>
        ))}
      </Flex>
    </section>
  );
}
