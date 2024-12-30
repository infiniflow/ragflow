import LLMLabel from '@/components/llm-select/llm-label';
import { useTheme } from '@/components/theme-provider';
import { IGenerateNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { useGetComponentLabelByValue } from '../../hooks/use-get-begin-query';
import { IGenerateParameter } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

export function GenerateNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IGenerateNode>) {
  const parameters: IGenerateParameter[] = get(data, 'form.parameters', []);
  const getLabel = useGetComponentLabelByValue(id);
  const { theme } = useTheme();
  return (
    <section
      className={classNames(
        styles.logicNode,
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
        style={RightHandleStyle}
        id="b"
      ></Handle>

      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        className={styles.nodeHeader}
      ></NodeHeader>

      <div className={styles.nodeText}>
        <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
      </div>
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
