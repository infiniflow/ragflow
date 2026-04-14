import { ReactFlowInstance } from '@xyflow/react';
import { useCallback } from 'react';
import {
  DROPDOWN_HORIZONTAL_OFFSET,
  DROPDOWN_VERTICAL_OFFSET,
  HALF_PLACEHOLDER_NODE_WIDTH,
} from '../constant';

/**
 * Dropdown position calculation Hook
 * Responsible for calculating dropdown menu position relative to placeholder node
 */
export const useDropdownPosition = (
  reactFlowInstance?: ReactFlowInstance<any, any>,
) => {
  /**
   * Calculate dropdown menu position
   * @param clientX Mouse click screen X coordinate
   * @param clientY Mouse click screen Y coordinate
   * @returns Dropdown menu screen coordinates
   */
  const calculateDropdownPosition = useCallback(
    (clientX: number, clientY: number) => {
      if (!reactFlowInstance) {
        return { x: clientX, y: clientY };
      }

      // Convert screen coordinates to flow coordinates
      const placeholderNodePosition = reactFlowInstance.screenToFlowPosition({
        x: clientX,
        y: clientY,
      });

      // Calculate dropdown position in flow coordinate system
      const dropdownFlowPosition = {
        x:
          placeholderNodePosition.x +
          HALF_PLACEHOLDER_NODE_WIDTH +
          DROPDOWN_HORIZONTAL_OFFSET,
        y: placeholderNodePosition.y - DROPDOWN_VERTICAL_OFFSET,
      };

      // Convert flow coordinates back to screen coordinates
      const dropdownScreenPosition =
        reactFlowInstance.flowToScreenPosition(dropdownFlowPosition);

      return {
        x: dropdownScreenPosition.x,
        y: dropdownScreenPosition.y,
      };
    },
    [reactFlowInstance],
  );

  /**
   * Calculate placeholder node flow coordinate position
   * @param clientX Mouse click screen X coordinate
   * @param clientY Mouse click screen Y coordinate
   * @returns Placeholder node flow coordinates
   */
  const getPlaceholderNodePosition = useCallback(
    (clientX: number, clientY: number) => {
      if (!reactFlowInstance) {
        return { x: clientX, y: clientY };
      }

      return reactFlowInstance.screenToFlowPosition({
        x: clientX,
        y: clientY,
      });
    },
    [reactFlowInstance],
  );

  /**
   * Convert flow coordinates to screen coordinates
   * @param flowPosition Flow coordinates
   * @returns Screen coordinates
   */
  const flowToScreenPosition = useCallback(
    (flowPosition: { x: number; y: number }) => {
      if (!reactFlowInstance) {
        return flowPosition;
      }

      return reactFlowInstance.flowToScreenPosition(flowPosition);
    },
    [reactFlowInstance],
  );

  /**
   * Convert screen coordinates to flow coordinates
   * @param screenPosition Screen coordinates
   * @returns Flow coordinates
   */
  const screenToFlowPosition = useCallback(
    (screenPosition: { x: number; y: number }) => {
      if (!reactFlowInstance) {
        return screenPosition;
      }

      return reactFlowInstance.screenToFlowPosition(screenPosition);
    },
    [reactFlowInstance],
  );

  return {
    calculateDropdownPosition,
    getPlaceholderNodePosition,
    flowToScreenPosition,
    screenToFlowPosition,
  };
};
