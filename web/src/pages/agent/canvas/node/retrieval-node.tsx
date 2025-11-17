import { NodeCollapsible } from '@/components/collapse';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { IRetrievalNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { CommonHandle, LeftEndHandle } from './handle';
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

  const { getLabel } = useGetVariableLabelOrTypeByValue(id);

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected}>
        <LeftEndHandle></LeftEndHandle>
        <CommonHandle
          id={NodeHandleId.Start}
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
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
        <NodeCollapsible items={knowledgeBaseIds}>
          {(id) => {
            const item = knowledgeList.find((y) => id === y.id);
            const label = getLabel(id);

            return (
              <div className={styles.nodeText} key={id}>
                <div className="flex items-center gap-1.5">
                  <RAGFlowAvatar
                    className="size-6 rounded-lg"
                    avatar={item?.avatar}
                    name={item ? item?.name : id}
                  />

                  <div className={'truncate flex-1'}>{label || item?.name}</div>
                </div>
              </div>
            );
          }}
        </NodeCollapsible>
      </NodeWrapper>
    </ToolBar>
  );
}

export const RetrievalNode = memo(InnerRetrievalNode);
