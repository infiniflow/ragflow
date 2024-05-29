import { useCallback, useEffect, useState } from 'react';
import ReactFlow, {
  Background,
  Controls,
  Edge,
  Node,
  NodeMouseHandler,
  OnConnect,
  OnEdgesChange,
  OnNodesChange,
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
} from 'reactflow';
import 'reactflow/dist/style.css';

import { NodeContextMenu, useHandleNodeContextMenu } from './context-menu';

import FlowDrawer from '../flow-drawer';
import {
  useHandleDrop,
  useHandleKeyUp,
  useHandleSelectionChange,
  useShowDrawer,
} from '../hooks';
import { dsl } from '../mock';
import { TextUpdaterNode } from './node';

const nodeTypes = { textUpdater: TextUpdaterNode };

interface IProps {
  sideWidth: number;
}

function FlowCanvas({ sideWidth }: IProps) {
  const [nodes, setNodes] = useState<Node[]>(dsl.graph.nodes);
  const [edges, setEdges] = useState<Edge[]>(dsl.graph.edges);

  const { selectedEdges, selectedNodes } = useHandleSelectionChange();

  const { ref, menu, onNodeContextMenu, onPaneClick } =
    useHandleNodeContextMenu(sideWidth);
  const { drawerVisible, hideDrawer, showDrawer } = useShowDrawer();

  const onNodesChange: OnNodesChange = useCallback(
    (changes) => setNodes((nds) => applyNodeChanges(changes, nds)),
    [],
  );
  const onEdgesChange: OnEdgesChange = useCallback(
    (changes) => setEdges((eds) => applyEdgeChanges(changes, eds)),
    [],
  );

  const onConnect: OnConnect = useCallback(
    (params) => setEdges((eds) => addEdge(params, eds)),
    [],
  );

  const onNodeClick: NodeMouseHandler = useCallback(() => {
    showDrawer();
  }, [showDrawer]);

  const { onDrop, onDragOver, setReactFlowInstance } = useHandleDrop(setNodes);

  const { handleKeyUp } = useHandleKeyUp(selectedEdges, selectedNodes);

  useEffect(() => {
    console.info('nodes:', nodes);
    console.info('edges:', edges);
  }, [nodes, edges]);

  return (
    <div style={{ height: '100%', width: '100%' }}>
      <ReactFlow
        ref={ref}
        nodes={nodes}
        onNodesChange={onNodesChange}
        onNodeContextMenu={onNodeContextMenu}
        edges={edges}
        onEdgesChange={onEdgesChange}
        fitView
        onConnect={onConnect}
        nodeTypes={nodeTypes}
        onPaneClick={onPaneClick}
        onDrop={onDrop}
        onDragOver={onDragOver}
        onNodeClick={onNodeClick}
        onInit={setReactFlowInstance}
        onKeyUp={handleKeyUp}
      >
        <Background />
        <Controls />
        {Object.keys(menu).length > 0 && (
          <NodeContextMenu onClick={onPaneClick} {...(menu as any)} />
        )}
      </ReactFlow>
      <FlowDrawer visible={drawerVisible} hideModal={hideDrawer}></FlowDrawer>
    </div>
  );
}

export default FlowCanvas;
