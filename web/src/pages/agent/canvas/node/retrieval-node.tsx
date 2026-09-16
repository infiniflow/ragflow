import { NodeCollapsible } from '@/components/collapse';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import {
  useFetchDatasetsByIds,
  useStaleDatasetIds,
} from '@/hooks/use-knowledge-request';
import { useFetchAllMemoryList } from '@/hooks/use-memory-request';
import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { get } from 'lodash';
import { TriangleAlert } from 'lucide-react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
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
  const knowledgeBaseIds: string[] = get(data, 'form.dataset_ids', []);
  const memoryIds: string[] = get(data, 'form.memory_ids', []);
  const { t } = useTranslation();

  const { getLabel } = useGetVariableLabelOrTypeByValue({ nodeId: id });

  const isMemory = data.form?.retrieval_from === RetrievalFrom.Memory;

  const persistedDatasetIds = isMemory ? [] : knowledgeBaseIds;

  // Resolve names/avatars for the persisted ids directly: the paginated
  // knowledge list can miss them (unloaded pages, emptied datasets filtered
  // out). Shares the byIds query with the staleness check below.
  const { data: datasets } = useFetchDatasetsByIds(persistedDatasetIds);

  // Mirror the form's stale-dataset validation: a persisted id referencing a
  // dataset that has since been deleted or emptied of chunks is flagged on
  // the node as well. Stays empty while the lookup is in flight.
  const { staleDatasetIds } = useStaleDatasetIds(persistedDatasetIds);

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

            const item = datasets?.find((y) => id === y.id);
            const label = getLabel(id);
            // A variable reference (has a label) is never stale; a dataset id
            // that no longer resolves to a usable dataset is.
            const isStale = !label && staleDatasetIds.has(id);

            return (
              <div
                className={classNames(styles.nodeText, {
                  'border border-state-error !bg-state-error-5': isStale,
                })}
                key={id}
                title={isStale ? t('chat.datasetUnavailable') : undefined}
              >
                <div className="flex items-center gap-1.5">
                  {isStale ? (
                    <TriangleAlert className="size-4 shrink-0 text-state-error" />
                  ) : (
                    <RAGFlowAvatar
                      className="size-6 rounded-lg"
                      avatar={item?.avatar}
                      name={item ? item?.name : id}
                    />
                  )}

                  <div className={'truncate flex-1'}>
                    {label || item?.name || id}
                  </div>
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
