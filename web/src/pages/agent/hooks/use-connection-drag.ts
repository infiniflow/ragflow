import { Connection, Position } from '@xyflow/react';
import { useCallback, useRef } from 'react';
import { useDropdownManager } from '../canvas/context';
import { Operator, PREVENT_CLOSE_DELAY } from '../constant';
import { useAddNode } from './use-add-node';

interface ConnectionStartParams {
  nodeId: string;
  handleId: string;
}

/**
 * Connection drag management Hook
 * Responsible for handling connection drag start and end logic
 */
export const useConnectionDrag = (
  reactFlowInstance: any,
  onConnect: (connection: Connection) => void,
  showModal: () => void,
  hideModal: () => void,
  setDropdownPosition: (position: { x: number; y: number }) => void,
  setCreatedPlaceholderRef: (nodeId: string | null) => void,
  calculateDropdownPosition: (
    clientX: number,
    clientY: number,
  ) => { x: number; y: number },
  removePlaceholderNode: () => void,
  clearActiveDropdown: () => void,
) => {
  // Reference for whether connection is established
  const isConnectedRef = useRef(false);
  // Reference for connection start parameters
  const connectionStartRef = useRef<ConnectionStartParams | null>(null);
  // Reference to prevent immediate close
  const preventCloseRef = useRef(false);
  // Reference to track mouse position for click detection
  const mouseStartPosRef = useRef<{ x: number; y: number } | null>(null);

  const { addCanvasNode } = useAddNode(reactFlowInstance);
  const { setActiveDropdown } = useDropdownManager();

  /**
   * Connection start handler function
   */
  const onConnectStart = useCallback((event: any, params: any) => {
    isConnectedRef.current = false;

    // Record mouse start position to detect click vs drag
    if ('clientX' in event && 'clientY' in event) {
      mouseStartPosRef.current = { x: event.clientX, y: event.clientY };
    }

    if (params && params.nodeId && params.handleId) {
      connectionStartRef.current = {
        nodeId: params.nodeId,
        handleId: params.handleId,
      };
    } else {
      connectionStartRef.current = null;
    }
  }, []);

  /**
   * Connection end handler function
   */
  const onConnectEnd = useCallback(
    (event: MouseEvent | TouchEvent) => {
      if ('clientX' in event && 'clientY' in event) {
        const { clientX, clientY } = event;
        setDropdownPosition({ x: clientX, y: clientY });

        if (!isConnectedRef.current && connectionStartRef.current) {
          // Check mouse movement distance to distinguish click from drag
          let isHandleClick = false;
          if (mouseStartPosRef.current) {
            const movementDistance = Math.sqrt(
              Math.pow(clientX - mouseStartPosRef.current.x, 2) +
                Math.pow(clientY - mouseStartPosRef.current.y, 2),
            );
            isHandleClick = movementDistance < 5; // Consider clicks within 5px as handle clicks
          }

          if (isHandleClick) {
            connectionStartRef.current = null;
            mouseStartPosRef.current = null;
            return;
          }
          // Create placeholder node and establish connection
          const mockEvent = { clientX, clientY };
          const contextData = {
            nodeId: connectionStartRef.current.nodeId,
            id: connectionStartRef.current.handleId,
            type: 'source' as const,
            position: Position.Right,
            isFromConnectionDrag: true,
          };

          // Use Placeholder operator to create node
          const newNodeId = addCanvasNode(
            Operator.Placeholder,
            contextData,
          )(mockEvent);

          // Record the created placeholder node ID
          if (newNodeId) {
            setCreatedPlaceholderRef(newNodeId);
          }

          // Calculate placeholder node position and display dropdown menu
          if (newNodeId && reactFlowInstance) {
            const dropdownScreenPosition = calculateDropdownPosition(
              clientX,
              clientY,
            );

            setDropdownPosition({
              x: dropdownScreenPosition.x,
              y: dropdownScreenPosition.y,
            });

            setActiveDropdown('drag');
            showModal();
            preventCloseRef.current = true;
            setTimeout(() => {
              preventCloseRef.current = false;
            }, PREVENT_CLOSE_DELAY);
          }

          // Reset connection state
          connectionStartRef.current = null;
          mouseStartPosRef.current = null;
        }
      }
    },
    [
      setDropdownPosition,
      addCanvasNode,
      setCreatedPlaceholderRef,
      reactFlowInstance,
      calculateDropdownPosition,
      setActiveDropdown,
      showModal,
    ],
  );

  /**
   * Connection establishment handler function
   */
  const handleConnect = useCallback(
    (connection: Connection) => {
      onConnect(connection);
      isConnectedRef.current = true;
    },
    [onConnect],
  );

  /**
   * Get connection start context data
   */
  const getConnectionStartContext = useCallback(() => {
    if (!connectionStartRef.current) {
      return null;
    }

    return {
      nodeId: connectionStartRef.current.nodeId,
      id: connectionStartRef.current.handleId,
      type: 'source' as const,
      position: Position.Right,
      isFromConnectionDrag: true,
    };
  }, []);

  /**
   * Check if close should be prevented
   */
  const shouldPreventClose = useCallback(() => {
    return preventCloseRef.current;
  }, []);

  /**
   * Handle canvas move/zoom events
   * Hide dropdown and remove placeholder when user scrolls or moves canvas
   */
  const onMove = useCallback(() => {
    // Clean up placeholder and dropdown when canvas moves/zooms
    removePlaceholderNode();
    hideModal();
    clearActiveDropdown();
  }, [removePlaceholderNode, hideModal, clearActiveDropdown]);

  return {
    onConnectStart,
    onConnectEnd,
    handleConnect,
    getConnectionStartContext,
    shouldPreventClose,
    onMove,
  };
};
