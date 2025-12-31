import { BaseNode } from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { NodeProps, Position } from '@xyflow/react';
import get from 'lodash/get';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AgentDialogueMode,
  BeginQueryType,
  BeginQueryTypeIconMap,
  NodeHandleId,
  Operator,
} from '../../constant';
import { BeginFormSchemaType } from '../../form/begin-form/schema';
import { useBuildWebhookUrl } from '../../hooks/use-build-webhook-url';
import { useIsPipeline } from '../../hooks/use-is-pipeline';
import OperatorIcon from '../../operator-icon';
import { LabelCard } from './card';
import { CommonHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import styles from './index.module.less';
import { NodeWrapper } from './node-wrapper';

function InnerBeginNode({
  data,
  id,
  selected,
}: NodeProps<BaseNode<BeginFormSchemaType>>) {
  const { t } = useTranslation();
  const inputs = get(data, 'form.inputs', {});

  const mode = data.form?.mode;

  const isWebhookMode = mode === AgentDialogueMode.Webhook;

  const url = useBuildWebhookUrl();

  const isPipeline = useIsPipeline();

  return (
    <NodeWrapper selected={selected} id={id}>
      <CommonHandle
        type="source"
        position={Position.Right}
        isConnectable
        style={RightHandleStyle}
        nodeId={id}
        id={NodeHandleId.Start}
      ></CommonHandle>

      <section className="flex items-center gap-2">
        <OperatorIcon name={data.label as Operator}></OperatorIcon>
        <div className="truncate text-center font-semibold text-sm">
          {t(`flow.begin`)}
        </div>
      </section>
      {isPipeline || (
        <div className="text-accent-primary mt-2 p-1 bg-bg-accent w-fit rounded-sm text-xs">
          {t(`flow.${isWebhookMode ? 'webhook.name' : mode}`)}
        </div>
      )}
      {isWebhookMode ? (
        <LabelCard className="mt-2 flex gap-1 items-center">
          <span className="font-bold">URL</span>
          <span className="flex-1 truncate">{url}</span>
        </LabelCard>
      ) : (
        <section
          className={cn(styles.generateParameters, 'flex gap-2 flex-col')}
        >
          {Object.entries(inputs).map(([key, val], idx) => {
            const Icon = BeginQueryTypeIconMap[val.type as BeginQueryType];
            return (
              <LabelCard key={idx} className={cn('flex gap-1.5 items-center')}>
                <Icon className="size-3.5" />
                <label
                  htmlFor=""
                  className="text-accent-primary text-sm italic"
                >
                  {key}
                </label>
                <LabelCard className="py-0.5 truncate flex-1">
                  {val.name}
                </LabelCard>
                <span className="flex-1">{val.optional ? 'Yes' : 'No'}</span>
              </LabelCard>
            );
          })}
        </section>
      )}
    </NodeWrapper>
  );
}

export const BeginNode = memo(InnerBeginNode);
