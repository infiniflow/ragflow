import { NodeCollapsible } from '@/components/collapse';
import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { NodeHandleId } from '../../constant';
import { ParserFormSchemaType } from '../../form/parser-form';
import { LabelCard } from './card';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';

function ParserNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<BaseNode<ParserFormSchemaType>>) {
  const { t } = useTranslation();
  return (
    <NodeWrapper selected={selected} id={id}>
      <CommonHandle
        id={NodeHandleId.End}
        type="target"
        position={Position.Left}
        isConnectable={isConnectable}
        style={LeftHandleStyle}
        nodeId={id}
      ></CommonHandle>
      <CommonHandle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        id={NodeHandleId.Start}
        style={RightHandleStyle}
        nodeId={id}
        isConnectableEnd={false}
      ></CommonHandle>
      <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>

      <NodeCollapsible items={data.form?.setups}>
        {(x, idx) => (
          <LabelCard
            key={idx}
            className="flex flex-col text-text-primary gap-1"
          >
            <span className="text-text-secondary">Parser {idx + 1}</span>
            {t(`flow.fileFormatOptions.${x.fileFormat}`)}
          </LabelCard>
        )}
      </NodeCollapsible>
    </NodeWrapper>
  );
}

export default memo(ParserNode);
