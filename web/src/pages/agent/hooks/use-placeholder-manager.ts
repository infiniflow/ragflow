import { pick } from 'lodash';
import { useCallback, useRef } from 'react';
import { Operator } from '../constant';
import useGraphStore from '../store';

/**
 * Placeholder node management Hook
 * Responsible for managing placeholder node creation, deletion, and state tracking
 */
export const usePlaceholderManager = (reactFlowInstance: any) => {
  // Reference to the created placeholder node ID
  const createdPlaceholderRef = useRef<string | null>(null);
  // Flag indicating whether user has selected a node
  const userSelectedNodeRef = useRef(false);

  /**
   * Check if placeholder node exists and remove it if found
   * Ensures only one placeholder can exist on the panel
   */
  const checkAndRemoveExistingPlaceholder = useCallback(() => {
    const { nodes, edges } = useGraphStore.getState();

    // Find existing placeholder node
    const existingPlaceholder = nodes.find(
      (node) => node.data?.label === Operator.Placeholder,
    );

    if (existingPlaceholder && reactFlowInstance) {
      // Remove edges related to placeholder
      const edgesToRemove = edges.filter(
        (edge) =>
          edge.target === existingPlaceholder.id ||
          edge.source === existingPlaceholder.id,
      );

      // Remove placeholder node
      const nodesToRemove = [existingPlaceholder];

      if (nodesToRemove.length > 0 || edgesToRemove.length > 0) {
        reactFlowInstance.deleteElements({
          nodes: nodesToRemove,
          edges: edgesToRemove,
        });
      }

      // Update ref reference
      if (createdPlaceholderRef.current === existingPlaceholder.id) {
        createdPlaceholderRef.current = null;
      }
    }
  }, [reactFlowInstance]);

  /**
   * Function to remove placeholder node
   * Called when user clicks blank area or cancels operation
   */
  const removePlaceholderNode = useCallback(() => {
    if (
      createdPlaceholderRef.current &&
      reactFlowInstance &&
      !userSelectedNodeRef.current
    ) {
      const { nodes, edges } = useGraphStore.getState();

      // Remove edges related to placeholder
      const edgesToRemove = edges.filter(
        (edge) =>
          edge.target === createdPlaceholderRef.current ||
          edge.source === createdPlaceholderRef.current,
      );

      // Remove placeholder node
      const nodesToRemove = nodes.filter(
        (node) => node.id === createdPlaceholderRef.current,
      );

      if (nodesToRemove.length > 0 || edgesToRemove.length > 0) {
        reactFlowInstance.deleteElements({
          nodes: nodesToRemove,
          edges: edgesToRemove,
        });
      }

      createdPlaceholderRef.current = null;
    }

    // Reset user selection flag
    userSelectedNodeRef.current = false;
  }, [reactFlowInstance]);

  /**
   * User node selection callback
   * Called when user selects a node type from dropdown menu
   */
  const onNodeCreated = useCallback(
    (newNodeId: string) => {
      // First establish connection between new node and source, then delete placeholder
      if (createdPlaceholderRef.current && reactFlowInstance) {
        const { nodes, edges, addEdge, updateNode } = useGraphStore.getState();

        // Find placeholder node to get its position
        const placeholderNode = nodes.find(
          (node) => node.id === createdPlaceholderRef.current,
        );

        // Find placeholder-related connection and get source node info
        const placeholderEdge = edges.find(
          (edge) => edge.target === createdPlaceholderRef.current,
        );

        // Update new node position to match placeholder position
        if (placeholderNode) {
          const newNode = nodes.find((node) => node.id === newNodeId);
          if (newNode) {
            updateNode({
              ...newNode,
              ...pick(placeholderNode, ['position', 'parentId', 'extent']),
            });
          }
        }

        if (placeholderEdge) {
          // Establish connection between new node and source node
          addEdge({
            source: placeholderEdge.source,
            target: newNodeId,
            sourceHandle: placeholderEdge.sourceHandle || null,
            targetHandle: placeholderEdge.targetHandle || null,
          });
        }

        // Remove placeholder node and related connections
        const edgesToRemove = edges.filter(
          (edge) =>
            edge.target === createdPlaceholderRef.current ||
            edge.source === createdPlaceholderRef.current,
        );

        const nodesToRemove = nodes.filter(
          (node) => node.id === createdPlaceholderRef.current,
        );

        if (nodesToRemove.length > 0 || edgesToRemove.length > 0) {
          reactFlowInstance.deleteElements({
            nodes: nodesToRemove,
            edges: edgesToRemove,
          });
        }
      }

      // Mark that user has selected a node
      userSelectedNodeRef.current = true;
      createdPlaceholderRef.current = null;
    },
    [reactFlowInstance],
  );

  /**
   * Set the created placeholder node ID
   */
  const setCreatedPlaceholderRef = useCallback((nodeId: string | null) => {
    createdPlaceholderRef.current = nodeId;
  }, []);

  /**
   * Reset user selection flag
   */
  const resetUserSelectedFlag = useCallback(() => {
    userSelectedNodeRef.current = false;
  }, []);

  return {
    removePlaceholderNode,
    onNodeCreated,
    setCreatedPlaceholderRef,
    resetUserSelectedFlag,
    checkAndRemoveExistingPlaceholder,
    createdPlaceholderRef: createdPlaceholderRef.current,
    userSelectedNodeRef: userSelectedNodeRef.current,
  };
};
