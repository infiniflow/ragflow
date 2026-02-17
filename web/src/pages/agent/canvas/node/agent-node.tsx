import { IAgentNode } from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentExceptionMethod, NodeHandleId } from '../../constant';
import { AgentFormSchemaType } from '../../form/agent-form';
import useGraphStore from '../../store';
import { hasSubAgent, isBottomSubAgent } from '../../utils';
import { LLMLabelCard } from './card';
import { CommonHandle, LeftEndHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerAgentNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IAgentNode<AgentFormSchemaType>>) {
  const edges = useGraphStore((state) => state.edges);
  const { t } = useTranslation();

  const isHeadAgent = useMemo(() => {
    return !isBottomSubAgent(edges, id);
  }, [edges, id]);

  const exceptionMethod = useMemo(() => {
    return get(data, 'form.exception_method');
  }, [data]);

  const hasTools = useMemo(() => {
    const tools = get(data, 'form.tools', []);
    const mcp = get(data, 'form.mcp', []);
    return tools.length > 0 || mcp.length > 0;
  }, [data]);

  const isGotoMethod = useMemo(() => {
    return exceptionMethod === AgentExceptionMethod.Goto;
  }, [exceptionMethod]);

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected} id={id}>
        {isHeadAgent && (
          <>
            <LeftEndHandle></LeftEndHandle>
            <CommonHandle
              type="source"
              position={Position.Right}
              isConnectable={isConnectable}
              style={RightHandleStyle}
              nodeId={id}
              id={NodeHandleId.Start}
              isConnectableEnd={false}
            ></CommonHandle>
          </>
        )}
        {isHeadAgent || (
          <Handle
            type="target"
            position={Position.Top}
            isConnectable={false}
            id={NodeHandleId.AgentTop}
            className="!bg-accent-primary !size-2"
          ></Handle>
        )}
        <Handle
          type="source"
          position={Position.Bottom}
          isConnectable={false}
          id={NodeHandleId.AgentBottom}
          style={{ left: 180 }}
          className={cn('!bg-accent-primary !size-2 invisible', {
            visible: hasSubAgent(edges, id),
          })}
        ></Handle>
        <Handle
          type="source"
          position={Position.Bottom}
          isConnectable={false}
          id={NodeHandleId.Tool}
          style={{ left: 20 }}
          className={cn('!bg-accent-primary !size-2 invisible', {
            visible: hasTools,
          })}
        ></Handle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
        <section className="flex flex-col gap-2">
          <LLMLabelCard llmId={get(data, 'form.llm_id')}></LLMLabelCard>
          {(isGotoMethod ||
            exceptionMethod === AgentExceptionMethod.Comment) && (
            <div className="bg-bg-card rounded-sm p-1 flex justify-between gap-2">
              <span className="text-text-secondary">{t('flow.onFailure')}</span>
              <span className="truncate flex-1 text-right">
                {t(`flow.${exceptionMethod}`)}
              </span>
            </div>
          )}
        </section>
        {isGotoMethod && (
          <CommonHandle
            type="source"
            position={Position.Right}
            isConnectable={isConnectable}
            className="!bg-state-error"
            style={{ ...RightHandleStyle, top: 94 }}
            nodeId={id}
            id={NodeHandleId.AgentException}
            isConnectableEnd={false}
          ></CommonHandle>
        )}
      </NodeWrapper>
    </ToolBar>
  );
}

export const AgentNode = memo(InnerAgentNode);
