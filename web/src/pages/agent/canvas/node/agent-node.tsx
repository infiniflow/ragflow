import LLMLabel from '@/components/llm-select/llm-label';
import { IAgentNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentExceptionMethod, NodeHandleId } from '../../constant';
import useGraphStore from '../../store';
import { isBottomSubAgent } from '../../utils';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerAgentNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IAgentNode>) {
  const edges = useGraphStore((state) => state.edges);
  const { t } = useTranslation();

  const isHeadAgent = useMemo(() => {
    return !isBottomSubAgent(edges, id);
  }, [edges, id]);

  const exceptionMethod = useMemo(() => {
    return get(data, 'form.exception_method');
  }, [data]);

  const isGotoMethod = useMemo(() => {
    return exceptionMethod === AgentExceptionMethod.Goto;
  }, [exceptionMethod]);

  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected}>
        {isHeadAgent && (
          <>
            <CommonHandle
              type="target"
              position={Position.Left}
              isConnectable={isConnectable}
              style={LeftHandleStyle}
              nodeId={id}
              id={NodeHandleId.End}
            ></CommonHandle>
            <CommonHandle
              type="source"
              position={Position.Right}
              isConnectable={isConnectable}
              className={styles.handle}
              style={RightHandleStyle}
              nodeId={id}
              id={NodeHandleId.Start}
              isConnectableEnd={false}
            ></CommonHandle>
          </>
        )}

        <Handle
          type="target"
          position={Position.Top}
          isConnectable={false}
          id={NodeHandleId.AgentTop}
        ></Handle>
        <Handle
          type="source"
          position={Position.Bottom}
          isConnectable={false}
          id={NodeHandleId.AgentBottom}
          style={{ left: 180 }}
        ></Handle>
        <Handle
          type="source"
          position={Position.Bottom}
          isConnectable={false}
          id={NodeHandleId.Tool}
          style={{ left: 20 }}
        ></Handle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
        <section className="flex flex-col gap-2">
          <div className={'bg-bg-card rounded-sm p-1'}>
            <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
          </div>
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
