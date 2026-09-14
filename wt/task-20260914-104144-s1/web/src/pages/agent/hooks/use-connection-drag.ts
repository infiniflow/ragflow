import {
  Connection,
  OnConnectEnd,
  OnConnectStart,
  Position,
  ReactFlowInstance,
} from '@xyflow/react';
import { useCallback, useRef } from 'react';
import { useDropdownManager } from '../canvas/context';
import { Operator, PREVENT_CLOSE_DELAY } from '../constant';
import useGraphStore from '../store';
import { useAddNode } from './use-add-node';
import { useIsPipeline } from './use-is-pipeline';

interface ConnectionStartParams {
  nodeId: string;
  handleId: string;
}

/**
 * Connection drag management Hook
 * Responsible for handling connection drag start and end logic
 */
export const useConnectionDrag = (
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
  checkAndRemoveExistingPlaceholder: () => void,
  reactFlowInstance?: ReactFlowInstance<any, any>,
) => {
  // Reference for whether connection is established
  const isConnectedRef = useRef(false);
  // Reference for connection start parameters
  const connectionStartRef = useRef<ConnectionStartParams | null>(null);
  // Reference to prevent immediate close
  const preventCloseRef = useRef(false);
  // Reference to track mouse position for click detection
  const mouseStartPosRef = useRef<{ x: number; y: number } | null>(null);
  // The next-step dropdown renders only after the drag ends, so the origin it
  // needs for filtering operators and wiring the new node must outlive the
  // drag itself.
  const pendingOriginRef = useRef<ConnectionStartParams | null>(null);

  const { addCanvasNode } = useAddNode(reactFlowInstance);
  const { setActiveDropdown } = useDropdownManager();
  const isPipeline = useIsPipeline();

  const onConnectStart: OnConnectStart = useCallback((event, params) => {
    isConnectedRef.current = false;
    pendingOriginRef.current = null;

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

  const onConnectEnd: OnConnectEnd = useCallback(
    (event) => {
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

          const abortConnection = () => {
            removePlaceholderNode();
            hideModal();
            clearActiveDropdown();
            connectionStartRef.current = null;
            mouseStartPosRef.current = null;
          };

          if (isHandleClick) {
            abortConnection();
            return;
          }

          // A pipeline node may only feed one downstream node, so a node that
          // already has a committed successor must not open another branch.
          if (
            isPipeline &&
            useGraphStore
              .getState()
              .hasDownstreamNode(connectionStartRef.current.nodeId)
          ) {
            abortConnection();
            return;
          }

          // Check and remove existing placeholder-node before creating new one
          checkAndRemoveExistingPlaceholder();

          pendingOriginRef.current = connectionStartRef.current;

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
      checkAndRemoveExistingPlaceholder,
      addCanvasNode,
      reactFlowInstance,
      removePlaceholderNode,
      hideModal,
      clearActiveDropdown,
      setCreatedPlaceholderRef,
      calculateDropdownPosition,
      setActiveDropdown,
      showModal,
      isPipeline,
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

  const getConnectionStartContext = useCallback(() => {
    if (!pendingOriginRef.current) {
      return null;
    }

    return {
      nodeId: pendingOriginRef.current.nodeId,
      id: pendingOriginRef.current.handleId,
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
    pendingOriginRef.current = null;
    removePlaceholderNode();
    hideModal();
    clearActiveDropdown();
  }, [removePlaceholderNode, hideModal, clearActiveDropdown]);

  return {
    nodeId: pendingOriginRef.current?.nodeId,
    onConnectStart,
    onConnectEnd,
    handleConnect,
    getConnectionStartContext,
    shouldPreventClose,
    onMove,
  };
};
