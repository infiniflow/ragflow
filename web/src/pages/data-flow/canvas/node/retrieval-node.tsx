import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { IRetrievalNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { useGetVariableLabelByValue } from '../../hooks/use-get-begin-query';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerRetrievalNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IRetrievalNode>) {
  const knowledgeBaseIds: string[] = get(data, 'form.kb_ids', []);
  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const getLabel = useGetVariableLabelByValue(id);

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected}>
        <CommonHandle
          id={NodeHandleId.End}
          type="target"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={LeftHandleStyle}
          nodeId={id}
        ></CommonHandle>
        <CommonHandle
          id={NodeHandleId.Start}
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          className={styles.handle}
          style={RightHandleStyle}
          nodeId={id}
          isConnectableEnd={false}
        ></CommonHandle>
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={classNames({
            [styles.nodeHeader]: knowledgeBaseIds.length > 0,
          })}
        ></NodeHeader>
        <section className="flex flex-col gap-2">
          {knowledgeBaseIds.map((id) => {
            const item = knowledgeList.find((y) => id === y.id);
            const label = getLabel(id);

            return (
              <div className={styles.nodeText} key={id}>
                <div className="flex items-center gap-1.5">
                  <RAGFlowAvatar
                    className="size-6 rounded-lg"
                    avatar={id}
                    name={item?.name || (label as string) || 'CN'}
                    isPerson={true}
                  />

                  <div className={'truncate flex-1'}>{label || item?.name}</div>
                </div>
              </div>
            );
          })}
        </section>
      </NodeWrapper>
    </ToolBar>
  );
}

export const RetrievalNode = memo(InnerRetrievalNode);
