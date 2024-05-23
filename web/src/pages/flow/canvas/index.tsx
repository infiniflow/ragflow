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
import { useHandleDrop, useShowDrawer } from '../hooks';
import { TextUpdaterNode } from './node';

const nodeTypes = { textUpdater: TextUpdaterNode };

const initialNodes = [
  {
    id: 'node-1',
    type: 'textUpdater',
    position: { x: 200, y: 50 },
    data: { value: 123 },
  },
  {
    id: '1',
    data: { label: 'Hello' },
    position: { x: 0, y: 0 },
    type: 'input',
  },
  {
    id: '2',
    data: { label: 'World' },
    position: { x: 100, y: 100 },
  },
];

const initialEdges = [
  { id: '1-2', source: '1', target: '2', label: 'to the', type: 'step' },
];

interface IProps {
  sideWidth: number;
  showDrawer(): void;
}

function FlowCanvas({ sideWidth }: IProps) {
  const [nodes, setNodes] = useState<Node[]>(initialNodes);
  const [edges, setEdges] = useState<Edge[]>(initialEdges);
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
