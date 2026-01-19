import { Panel, type NodeProps, type PanelPosition } from '@xyflow/react';
import { type ComponentProps, type ReactNode } from 'react';

import { BaseNode } from '@/components/xyflow/base-node';
import { cn } from '@/lib/utils';

/* GROUP NODE Label ------------------------------------------------------- */

export type GroupNodeLabelProps = ComponentProps<'div'>;

export function GroupNodeLabel({
  children,
  className,
  ...props
}: GroupNodeLabelProps) {
  return (
    <div className="h-full w-full" {...props}>
      <div
        className={cn(
          'text-card-foreground bg-secondary w-fit p-2 text-xs',
          className,
        )}
      >
        {children}
      </div>
    </div>
  );
}

export type GroupNodeProps = Partial<NodeProps> & {
  label?: ReactNode;
  position?: PanelPosition;
};

/* GROUP NODE -------------------------------------------------------------- */

export function LabeledGroupNode({
  label = '',
  position,
  ...props
}: GroupNodeProps) {
  const getLabelClassName = (position?: PanelPosition) => {
    switch (position) {
      case 'top-left':
        return 'rounded-br-sm';
      case 'top-center':
        return 'rounded-b-sm';
      case 'top-right':
        return 'rounded-bl-sm';
      case 'bottom-left':
        return 'rounded-tr-sm';
      case 'bottom-right':
        return 'rounded-tl-sm';
      case 'bottom-center':
        return 'rounded-t-sm';
      default:
        return 'rounded-br-sm';
    }
  };

  return (
    <BaseNode
      className="bg-opacity-50 h-full overflow-hidden rounded-sm"
      {...props}
    >
      <Panel className="m-0 p-0" position={position}>
        {label && (
          <GroupNodeLabel className={getLabelClassName(position)}>
            {label}
          </GroupNodeLabel>
        )}
      </Panel>
    </BaseNode>
  );
}
