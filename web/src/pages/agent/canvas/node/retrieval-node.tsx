import { NodeCollapsible } from '@/components/collapse';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import { useFetchAllMemoryList } from '@/hooks/use-memory-request';
import { BaseNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId, RetrievalFrom } from '../../constant';
import { RetrievalFormSchemaType } from '../../form/retrieval-form/next';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { LabelCard } from './card';
import { CommonHandle, LeftEndHandle } from './handle';
import styles from './index.module.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerRetrievalNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<BaseNode<RetrievalFormSchemaType>>) {
  const knowledgeBaseIds: string[] = get(data, 'form.kb_ids', []);
  const memoryIds: string[] = get(data, 'form.memory_ids', []);
  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const { getLabel } = useGetVariableLabelOrTypeByValue({ nodeId: id });

  const isMemory = data.form?.retrieval_from === RetrievalFrom.Memory;

  const memoryList = useFetchAllMemoryList();

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected} id={id}>
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
        <NodeCollapsible items={isMemory ? memoryIds : knowledgeBaseIds}>
          {(id) => {
            if (isMemory) {
              const item = memoryList.data?.find((y) => id === y.id);
              return (
                <LabelCard key={id} className="flex items-center gap-1.5">
                  <RAGFlowAvatar
                    className="size-6 rounded-lg"
                    avatar={item?.avatar ?? ''}
                    name={item ? item?.name : id}
                  />
                  <span className="flex-1 truncate"> {item?.name}</span>
                </LabelCard>
              );
            }

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
