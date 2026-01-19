import { NodeProps, NodeToolbar, NodeToolbarProps } from '@xyflow/react';
import {
  HTMLAttributes,
  ReactNode,
  createContext,
  forwardRef,
  useCallback,
  useContext,
  useState,
} from 'react';
import { BaseNode } from './base-node';

/* TOOLTIP CONTEXT ---------------------------------------------------------- */

const TooltipContext = createContext(false);

/* TOOLTIP NODE ------------------------------------------------------------- */

export type TooltipNodeProps = Partial<NodeProps> & {
  children?: ReactNode;
};

/**
 * A component that wraps a node and provides tooltip visibility context.
 */
export const TooltipNode = forwardRef<HTMLDivElement, TooltipNodeProps>(
  ({ selected, children }, ref) => {
    const [isTooltipVisible, setTooltipVisible] = useState(false);

    const showTooltip = useCallback(() => setTooltipVisible(true), []);
    const hideTooltip = useCallback(() => setTooltipVisible(false), []);

    return (
      <TooltipContext.Provider value={isTooltipVisible}>
        <BaseNode
          ref={ref}
          onMouseEnter={showTooltip}
          onMouseLeave={hideTooltip}
          onFocus={showTooltip}
          onBlur={hideTooltip}
          tabIndex={0}
          selected={selected}
          className="h-full bg-transparent"
        >
          {children}
        </BaseNode>
      </TooltipContext.Provider>
    );
  },
);

TooltipNode.displayName = 'TooltipNode';

/* TOOLTIP CONTENT ---------------------------------------------------------- */

export type TooltipContentProps = NodeToolbarProps;

/**
 * A component that displays the tooltip content based on visibility context.
 */
export const TooltipContent = forwardRef<HTMLDivElement, TooltipContentProps>(
  ({ position, children }, ref) => {
    const isTooltipVisible = useContext(TooltipContext);

    return (
      <div ref={ref}>
        <NodeToolbar
          isVisible={isTooltipVisible}
          className=" bg-transparent  text-primary-foreground"
          tabIndex={1}
          position={position}
          offset={0}
          align={'end'}
        >
          {children}
        </NodeToolbar>
      </div>
    );
  },
);

TooltipContent.displayName = 'TooltipContent';

/* TOOLTIP TRIGGER ---------------------------------------------------------- */

export type TooltipTriggerProps = HTMLAttributes<HTMLParagraphElement>;

/**
 * A component that triggers the tooltip visibility.
 */
export const TooltipTrigger = forwardRef<
  HTMLParagraphElement,
  TooltipTriggerProps
>(({ children, ...props }, ref) => {
  return (
    <div ref={ref} {...props}>
      {children}
    </div>
  );
});

TooltipTrigger.displayName = 'TooltipTrigger';
