import { NodeCollapsible } from '@/components/collapse';
import { IMessageNode } from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { useGetVariableLabelOrTypeByValue } from '@/pages/agent/hooks/use-get-begin-query';
import { NodeProps } from '@xyflow/react';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { LabelCard } from './card';
import { LeftEndHandle } from './handle';
import styles from './index.module.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';
import { VariableDisplay } from './variable-display';

function InnerMessageNode({ id, data, selected }: NodeProps<IMessageNode>) {
  const messages: string[] = get(data, 'form.content', []);
  const { getLabel } = useGetVariableLabelOrTypeByValue({ nodeId: id });
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected} id={id}>
        <LeftEndHandle></LeftEndHandle>
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
