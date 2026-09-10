import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { NodeHandleId } from '../../constant';
import { ParserFormSchemaType } from '../../form/parser-form';
import { CommonHandle } from './handle';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { LabelCard } from './card';

function ParserNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<BaseNode<ParserFormSchemaType>>) {
  const { t } = useTranslation();

  const fileFormats = (data.form?.setups ?? [])
    .map((setup) => setup.fileFormat)
    .filter((format): format is string => typeof format === 'string');

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
      {fileFormats.length > 0 && (
        <LabelCard className="flex flex-wrap gap-2">
          {fileFormats.map((format, index) => (
            <span key={index} className="italic text-text-secondary">
              {t(`flow.fileFormatOptions.${format}`)}
            </span>
          ))}
        </LabelCard>
      )}
    </NodeWrapper>
  );
}

export default memo(ParserNode);
