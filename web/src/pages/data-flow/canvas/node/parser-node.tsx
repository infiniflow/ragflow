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
    <NodeWrapper selected={selected}>
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
      <section className="space-y-2">
        {data.form?.setups.map((x, idx) => (
          <LabelCard
            key={idx}
            className="flex justify-between text-text-primary"
          >
            <span className="text-text-secondary">Parser {idx + 1}</span>
            {t(`dataflow.fileFormatOptions.${x.fileFormat}`)}
          </LabelCard>
        ))}
      </section>
    </NodeWrapper>
  );
}

export default memo(ParserNode);
