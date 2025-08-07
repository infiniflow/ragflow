import { Card, CardContent } from '@/components/ui/card';
import { ISwitchCondition, ISwitchNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { memo, useCallback } from 'react';
import { NodeHandleId, SwitchOperatorOptions } from '../../constant';
import { LogicalOperatorIcon } from '../../form/switch-form';
import { useGetVariableLabelByValue } from '../../hooks/use-get-begin-query';
import { CommonHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';
import { useBuildSwitchHandlePositions } from './use-build-switch-handle-positions';

const getConditionKey = (idx: number, length: number) => {
  if (idx === 0 && length !== 1) {
    return 'If';
  } else if (idx === length - 1) {
    return 'Else';
  }

  return 'ElseIf';
};

const ConditionBlock = ({
  condition,
  nodeId,
}: { condition: ISwitchCondition } & { nodeId: string }) => {
  const items = condition?.items ?? [];
  const getLabel = useGetVariableLabelByValue(nodeId);

  const renderOperatorIcon = useCallback((operator?: string) => {
    const item = SwitchOperatorOptions.find((x) => x.value === operator);
    if (item) {
      return (
        <LogicalOperatorIcon
          icon={item?.icon}
          value={item?.value}
        ></LogicalOperatorIcon>
      );
    }
    return <></>;
  }, []);

  return (
    <Card>
      <CardContent className="p-0 divide-y divide-background-card">
        {items.map((x, idx) => (
          <div key={idx}>
            <section className="flex justify-between gap-2 items-center text-xs p-1">
              <div className="flex-1 truncate text-accent-primary">
                {getLabel(x?.cpn_id)}
              </div>
              <span>{renderOperatorIcon(x?.operator)}</span>
              <div className="flex-1 truncate">{x?.value}</div>
            </section>
          </div>
        ))}
      </CardContent>
    </Card>
  );
};

function InnerSwitchNode({ id, data, selected }: NodeProps<ISwitchNode>) {
  const { positions } = useBuildSwitchHandlePositions({ data, id });
  return (
    <ToolBar selected={selected} id={id} label={data.label} showRun={false}>
      <NodeWrapper selected={selected}>
        <CommonHandle
          type="target"
          position={Position.Left}
          isConnectable
          nodeId={id}
          id={NodeHandleId.End}
        ></CommonHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
        <section className="gap-2.5 flex flex-col">
          {positions.map((position, idx) => {
            return (
              <div key={idx}>
                <section className="flex flex-col text-xs">
                  <div className="text-right">
                    <span>{getConditionKey(idx, positions.length)}</span>
                    <div className="text-text-secondary">
                      {idx < positions.length - 1 && position.text}
                    </div>
                  </div>
                  <span className="text-accent-primary">
                    {idx < positions.length - 1 &&
                      position.condition?.logical_operator?.toUpperCase()}
                  </span>
                  {position.condition && (
                    <ConditionBlock
                      condition={position.condition}
                      nodeId={id}
                    ></ConditionBlock>
                  )}
                </section>
                <CommonHandle
                  key={position.text}
                  id={position.text}
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

export const SwitchNode = memo(InnerSwitchNode);
