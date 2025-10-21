import { BaseNode } from '@/interfaces/database/agent';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { NodeHandleId } from '../../constant';
import { TokenizerFormSchemaType } from '../../form/tokenizer-form';
import { LabelCard } from './card';
import { CommonHandle } from './handle';
import { LeftHandleStyle } from './handle-icon';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function TokenizerNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<BaseNode<TokenizerFormSchemaType>>) {
  const { t } = useTranslation();

  return (
    <ToolBar
      selected={selected}
      id={id}
      label={data.label}
      showRun={false}
      showCopy={false}
    >
      <NodeWrapper selected={selected}>
        <CommonHandle
          id={NodeHandleId.End}
          type="target"
          position={Position.Left}
          isConnectable={isConnectable}
          style={LeftHandleStyle}
          nodeId={id}
        ></CommonHandle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
        <LabelCard className="text-text-primary flex justify-between flex-col gap-1">
          <span className="text-text-secondary">{t('flow.searchMethod')}</span>
          <ul className="space-y-1">
            {data.form?.search_method.map((x) => (
              <li key={x}>{t(`flow.tokenizerSearchMethodOptions.${x}`)}</li>
            ))}
          </ul>
        </LabelCard>
      </NodeWrapper>
    </ToolBar>
  );
}

export default memo(TokenizerNode);
