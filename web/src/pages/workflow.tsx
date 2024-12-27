import { useCallback } from 'react';
import ReactFlow, {
  Background,
  Controls,
  Handle,
  MiniMap,
  NodeProps,
  Position,
  addEdge,
  useEdgesState,
  useNodesState,
} from 'reactflow';
import 'reactflow/dist/style.css';

import './workflow.less';

const initialNodes = [
  {
    id: '1',
    type: 'input',
    data: { label: 'Node 0' },
    position: { x: 250, y: 5 },
    className: 'light',
  },
  {
    id: '2',
    data: { label: 'Group A' },
    position: { x: 100, y: 100 },
    className: 'light',
    style: { backgroundColor: 'rgba(255, 0, 0, 0.2)', width: 200, height: 200 },
  },
  {
    id: '2a',
    data: { label: 'Node A.1' },
    position: { x: 10, y: 50 },
    parentId: '2',
  },
  {
    id: '3',
    data: { label: 'Node 1' },
    position: { x: 320, y: 100 },
    className: 'light',
  },
  {
    id: '4',
    data: { label: 'Group B' },
    position: { x: 320, y: 200 },
    className: 'light',
    style: { backgroundColor: 'rgba(255, 0, 0, 0.2)', width: 300, height: 300 },
    type: 'group',
  },
  {
    id: '4a',
    data: { label: 'Node B.1' },
    position: { x: 15, y: 65 },
    className: 'light',
    parentId: '4',
    extent: 'parent',
    draggable: false,
  },
  {
    id: '4b',
    data: { label: 'Group B.A' },
    position: { x: 15, y: 120 },
    className: 'light',
    style: {
      backgroundColor: 'rgba(255, 0, 255, 0.2)',
      height: 150,
      width: 270,
    },
    parentId: '4',
  },
  {
    id: '4b1',
    data: { label: 'Node B.A.1' },
    position: { x: 20, y: 40 },
    className: 'light',
    parentId: '4b',
  },
  {
    id: '4b2',
    data: { label: 'Node B.A.2' },
    position: { x: 100, y: 100 },
    className: 'light',
    parentId: '4b',
  },
];

const initialEdges = [
  { id: 'e1-2', source: '1', target: '2', animated: true },
  { id: 'e1-3', source: '1', target: '3' },
  { id: 'e2a-4a', source: '2a', target: '4a' },
  { id: 'e3-4b', source: '3', target: '4b' },
  { id: 'e4a-4b1', source: '4a', target: '4b1' },
  { id: 'e4a-4b2', source: '4a', target: '4b2' },
  { id: 'e4b1-4b2', source: '4b1', target: '4b2' },
];

export function RagNode({ id, data, isConnectable = true }: NodeProps<any>) {
  return (
    <section className="ragflow-group w-full h-full">
      <div className="h-10 bg-slate-200 text-orange-400">header</div>
      <Handle
        id="c"
        type="source"
        position={Position.Left}
        isConnectable={isConnectable}
      ></Handle>
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        id="b"
      ></Handle>
      <div className="w-full h-10">xxx</div>
    </section>
  );
}

const nodeTypes = { group: RagNode };

const NestedFlow = () => {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  const onConnect = useCallback((connection) => {
    setEdges((eds) => addEdge(connection, eds));
  }, []);

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      className="react-flow-subflows-example"
      fitView
      onNodeClick={(node) => {
        console.log(node);
      }}
      nodeTypes={nodeTypes}
    >
      <MiniMap />
      <Controls />
      <Background />
    </ReactFlow>
  );
};

export default NestedFlow;
