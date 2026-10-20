import { LLMLabelCard } from './card';
import { CommonHandle, LeftEndHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import {
  ScoredSwitchEscalateHandleId,
  showsScoredSwitchEscalateHandle,
} from '@/pages/agent/scored-switch';
import useGraphStore from '@/pages/agent/store';
import { Position, useUpdateNodeInternals } from '@xyflow/react';
import { get } from 'lodash';
import { memo, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

type OptionItem = {
  name?: string;
  uuid?: string;
};

function optionTop(index: number, previousTop: number) {
  if (index === 0) {
    return 86;
  }
  return previousTop + 32;
}

function InnerScoredSwitchNode({
  id,
  data,
  selected,
}: {
  id: string;
  data: RAGFlowNodeType['data'];
  selected?: boolean;
}) {
  const { t } = useTranslation();
  const updateNodeInternals = useUpdateNodeInternals();
  const deleteEdges = useGraphStore(
    (state) => state.deleteEdgesBySourceAndSourceHandle,
  );
  const items = useMemo(
    () => (get(data, 'form.items', []) ?? []) as OptionItem[],
    [data],
  );
  const fieldMode = get(data, 'form.field_mode') === true;
  const showEscalate =
    !fieldMode &&
    showsScoredSwitchEscalateHandle(get(data, 'form.escalate_enabled'));
  const positions = useMemo(() => {
    const list: Array<{ top: number; name: string; uuid: string }> = [];
    if (fieldMode) {
      return list;
    }
    items.forEach((item, index) => {
      const previous = list[index - 1]?.top ?? 0;
      list.push({
        top: optionTop(index, previous),
        name: item?.name || '',
        uuid: item?.uuid || '',
      });
    });
    return list;
  }, [fieldMode, items]);
  const escalateTop =
    positions.length === 0 ? 86 : positions[positions.length - 1].top + 32;

  useEffect(() => {
    if (fieldMode) {
      items.forEach((item) => {
        if (item?.uuid) {
          deleteEdges(id, item.uuid);
        }
      });
      deleteEdges(id, ScoredSwitchEscalateHandleId);
    } else if (!showEscalate) {
      deleteEdges(id, ScoredSwitchEscalateHandleId);
    }
    updateNodeInternals(id);
  }, [deleteEdges, fieldMode, id, items, showEscalate, updateNodeInternals]);

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected} id={id}>
        <LeftEndHandle></LeftEndHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
        <section className="flex flex-col gap-2">
          <LLMLabelCard llmId={get(data, 'form.llm_id')}></LLMLabelCard>
          {positions.map((position) => (
            <div key={position.uuid || position.name}>
              <div className="bg-bg-card rounded-sm p-1 truncate">
                {position.name}
              </div>
              <CommonHandle
                id={position.uuid}
                type="source"
                position={Position.Right}
                isConnectable
                style={{ ...RightHandleStyle, top: position.top }}
                nodeId={id}
                isConnectableEnd={false}
              ></CommonHandle>
            </div>
          ))}
          {showEscalate && (
            <div>
              <div className="bg-bg-card rounded-sm p-1 truncate text-text-secondary">
                {t('flow.scoredSwitchEscalate')}
              </div>
              <CommonHandle
                id={ScoredSwitchEscalateHandleId}
                type="source"
                position={Position.Right}
                isConnectable
                style={{ ...RightHandleStyle, top: escalateTop }}
                nodeId={id}
                isConnectableEnd={false}
              ></CommonHandle>
            </div>
          )}
        </section>
      </NodeWrapper>
    </ToolBar>
  );
}

export const ScoredSwitchNode = memo(InnerScoredSwitchNode);
