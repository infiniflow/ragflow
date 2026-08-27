/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
