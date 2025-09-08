import LLMLabel from '@/components/llm-select/llm-label';
import { ICategorizeNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { CommonHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';
import { useBuildCategorizeHandlePositions } from './use-build-categorize-handle-positions';

export function InnerCategorizeNode({
  id,
  data,
  selected,
}: NodeProps<ICategorizeNode>) {
  const { positions } = useBuildCategorizeHandlePositions({ data, id });
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected}>
        <CommonHandle
          type="target"
          position={Position.Left}
          isConnectable
          id={NodeHandleId.End}
          nodeId={id}
        ></CommonHandle>

        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>

        <section className="flex flex-col gap-2">
          <div className={'bg-bg-card rounded-sm px-1'}>
            <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
          </div>
          {positions.map((position) => {
            return (
              <div key={position.uuid}>
                <div className={'bg-bg-card rounded-sm p-1 truncate'}>
                  {position.name}
                </div>
                <CommonHandle
                  // key={position.text}
                  id={position.uuid}
                  type="source"
                  position={Position.Right}
                  isConnectable
                  style={{ ...RightHandleStyle, top: position.top }}
                  nodeId={id}
                  isConnectableEnd={false}
                ></CommonHandle>
              </div>
            );
          })}
        </section>
      </NodeWrapper>
    </ToolBar>
  );
}

export const CategorizeNode = memo(InnerCategorizeNode);
