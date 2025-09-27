import { Connection, Position } from '@xyflow/react';
import { useCallback, useRef } from 'react';
import { useDropdownManager } from '../canvas/context';
import { Operator, PREVENT_CLOSE_DELAY } from '../constant';
import { useAddNode } from './use-add-node';

interface ConnectionStartParams {
  nodeId: string;
  handleId: string;
  startX?: number;
  startY?: number;
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

  const DRAG_THRESHOLD = 5;

  const { addCanvasNode } = useAddNode(reactFlowInstance);
  const { setActiveDropdown } = useDropdownManager();

  /**
   * Connection start handler function
   */
  const onConnectStart = useCallback((event: any, params: any) => {
    console.log('[DEBUG] onConnectStart:', {
      nodeId: params?.nodeId,
      handleId: params?.handleId,
      clientX: event?.clientX,
      clientY: event?.clientY,
    });
    isConnectedRef.current = false;

    if (params && params.nodeId && params.handleId) {
      const startX = event?.clientX || 0;
      const startY = event?.clientY || 0;

      connectionStartRef.current = {
        nodeId: params.nodeId,
        handleId: params.handleId,
        startX,
        startY,
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
      console.log('[DEBUG] onConnectEnd triggered');
      if ('clientX' in event && 'clientY' in event) {
        const { clientX, clientY } = event;

        if (!isConnectedRef.current && connectionStartRef.current) {
          const startX = connectionStartRef.current.startX || 0;
          const startY = connectionStartRef.current.startY || 0;
          const dragDistance = Math.sqrt(
            Math.pow(clientX - startX, 2) + Math.pow(clientY - startY, 2),
          );

          console.log('[DEBUG] dragDistance:', {
            dragDistance,
            threshold: DRAG_THRESHOLD,
            isDrag: dragDistance > DRAG_THRESHOLD,
          });

          if (dragDistance > DRAG_THRESHOLD) {
            console.log('[DEBUG] Creating placeholder node (drag detected)');
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
          } else {
            console.log('[DEBUG] Showing dropdown directly (click detected)');
            setDropdownPosition({ x: clientX, y: clientY });
            setActiveDropdown('handle');
            showModal();
            preventCloseRef.current = true;
            setTimeout(() => {
              preventCloseRef.current = false;
            }, PREVENT_CLOSE_DELAY);
          }

          // Reset connection state
          connectionStartRef.current = null;
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
      DRAG_THRESHOLD,
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
