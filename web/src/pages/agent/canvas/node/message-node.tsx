import { NodeCollapsible } from '@/components/collapse';
import { IMessageNode } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { useGetVariableLabelOrTypeByValue } from '@/pages/agent/hooks/use-get-begin-query';
import { Handle, NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { LabelCard } from './card';
import { LeftEndHandle } from './handle';
import styles from './index.module.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';
import { VariableDisplay } from './variable-display';

function InnerMessageNode({
  id,
  data,
  selected,
  isConnectable = true,
}: NodeProps<IMessageNode>) {
  const messages: string[] = get(data, 'form.content', []);
  const { getLabel } = useGetVariableLabelOrTypeByValue({ nodeId: id });
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected} id={id}>
        <LeftEndHandle></LeftEndHandle>
        {/* v1 Message/Answer nodes are routable: they have a downstream
            too, so they need a source handle on the right with the
            "start" id that edges' sourceHandle field references. The
            original component only rendered a target handle, which
            silently dropped any edge with this node as the source. */}
        <Handle
          type="source"
          id={NodeHandleId.Start}
          position={Position.Right}
          isConnectable={isConnectable}
          className="!bg-accent-primary !size-2"
        />
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={classNames({
            [styles.nodeHeader]: messages.length > 0,
          })}
        ></NodeHeader>
        <section
          className={cn('flex flex-col gap-2', styles.messageNodeContainer)}
        >
          <NodeCollapsible items={messages}>
            {(x, idx) => (
              <LabelCard key={idx} className="truncate">
                <VariableDisplay content={x} getLabel={getLabel} />
              </LabelCard>
            )}
          </NodeCollapsible>
        </section>
      </NodeWrapper>
    </ToolBar>
  );
}

export const MessageNode = memo(InnerMessageNode);
