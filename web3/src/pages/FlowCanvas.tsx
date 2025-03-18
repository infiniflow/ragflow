import React, { useState, useRef, useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

type BlockType = 'agent' | 'message' | 'email' | 'google'
type Position = { x: number; y: number }

interface Block {
  id: string
  type: BlockType
  position: Position
  title: string
}

interface Connection {
  id: string
  source: string
  target: string
}

const FlowCanvas = () => {
  const location = useLocation()
  const navigate = useNavigate()
  const [blocks, setBlocks] = useState<Block[]>([])
  const [connections, setConnections] = useState<Connection[]>([])
  const [selectedBlock, setSelectedBlock] = useState<string | null>(null)
  const [draggingBlock, setDraggingBlock] = useState<{ id: string, offsetX: number, offsetY: number } | null>(null)
  const [connectingFrom, setConnectingFrom] = useState<string | null>(null)
  const [templateName, setTemplateName] = useState('')
  const [mousePosX, setMousePosX] = useState(0)
  const [mousePosY, setMousePosY] = useState(0)
  const canvasRef = useRef<HTMLDivElement>(null)
  
  // Get the template name from location state
  useEffect(() => {
    if (location.state?.templateName) {
      setTemplateName(location.state.templateName)
    } else {
      // If no template was selected, redirect back to flow designer
      navigate('/flow-designer')
    }
  }, [location, navigate])

  const handleDragStart = (e: React.DragEvent, blockType: BlockType) => {
    e.dataTransfer.setData('blockType', blockType)
    e.dataTransfer.effectAllowed = 'copy'
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const blockType = e.dataTransfer.getData('blockType') as BlockType
    
    if (!blockType || !canvasRef.current) return
    
    const canvasRect = canvasRef.current.getBoundingClientRect()
    const x = e.clientX - canvasRect.left
    const y = e.clientY - canvasRect.top
    
    const newBlock: Block = {
      id: `block-${Date.now()}`,
      type: blockType,
      position: { x, y },
      title: getBlockTitle(blockType)
    }
    
    setBlocks([...blocks, newBlock])
  }

  const getBlockTitle = (blockType: BlockType): string => {
    switch (blockType) {
      case 'agent': return 'AI Agent'
      case 'message': return 'Message'
      case 'email': return 'Email'
      case 'google': return 'Google Search'
    }
  }

  const getBlockIcon = (blockType: BlockType): JSX.Element => {
    switch (blockType) {
      case 'agent':
        return (
          <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
        )
      case 'message':
        return (
          <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" />
          </svg>
        )
      case 'email':
        return (
          <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
        )
      case 'google':
        return (
          <svg className="w-6 h-6" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path d="M12.48 10.92v3.28h7.84c-.24 1.84-.853 3.187-1.787 4.133-1.147 1.147-2.933 2.4-6.053 2.4-4.827 0-8.6-3.893-8.6-8.72s3.773-8.72 8.6-8.72c2.6 0 4.507 1.027 5.907 2.347l2.307-2.307C18.747 1.44 16.133 0 12.48 0 5.867 0 .307 5.387.307 12s5.56 12 12.173 12c3.573 0 6.267-1.173 8.373-3.36 2.16-2.16 2.84-5.213 2.84-7.667 0-.76-.053-1.467-.173-2.053H12.48z" fill="#4285F4"/>
            <path d="M6.52 13.12c-1.36-1.36-1.36-3.56 0-4.92 1.36-1.36 3.56-1.36 4.92 0 1.36 1.36 1.36 3.56 0 4.92-1.36 1.36-3.56 1.36-4.92 0z" fill="#EA4335"/>
            <path d="M17.48 13.12c-1.36 1.36-3.56 1.36-4.92 0v-4.92h4.92c1.36 1.36 1.36 3.56 0 4.92z" fill="#FBBC05"/>
            <path d="M17.48 8.2c1.36 1.36 1.36 3.56 0 4.92-1.36 1.36-3.56 1.36-4.92 0V8.2h4.92z" fill="#34A853"/>
          </svg>
        )
    }
  }

  const handleBlockMouseDown = (e: React.MouseEvent, blockId: string) => {
    if (e.button !== 0) return // Only left click
    
    const block = blocks.find(b => b.id === blockId)
    if (!block) return
    
    setSelectedBlock(blockId)
    setDraggingBlock({
      id: blockId,
      offsetX: e.clientX - block.position.x,
      offsetY: e.clientY - block.position.y
    })
  }

  const handleCanvasMouseMove = (e: React.MouseEvent) => {
    setMousePosX(e.clientX)
    setMousePosY(e.clientY)
    
    if (!draggingBlock || !canvasRef.current) return
    
    const canvasRect = canvasRef.current.getBoundingClientRect()
    const x = e.clientX - canvasRect.left - draggingBlock.offsetX
    const y = e.clientY - canvasRect.top - draggingBlock.offsetY
    
    setBlocks(blocks.map(block => 
      block.id === draggingBlock.id
        ? { ...block, position: { x, y } }
        : block
    ))
  }

  const handleCanvasMouseUp = () => {
    setDraggingBlock(null)
  }

  const handleConnectorMouseDown = (e: React.MouseEvent, blockId: string) => {
    e.stopPropagation()
    setConnectingFrom(blockId)
  }

  const handleBlockMouseUp = (blockId: string) => {
    if (connectingFrom && connectingFrom !== blockId) {
      // Check if connection already exists
      const connectionExists = connections.some(
        conn => conn.source === connectingFrom && conn.target === blockId
      )
      
      if (!connectionExists) {
        const newConnection: Connection = {
          id: `conn-${Date.now()}`,
          source: connectingFrom,
          target: blockId
        }
        setConnections([...connections, newConnection])
      }
    }
    setConnectingFrom(null)
  }

  const handleDeleteBlock = (blockId: string) => {
    setBlocks(blocks.filter(block => block.id !== blockId))
    setConnections(connections.filter(
      conn => conn.source !== blockId && conn.target !== blockId
    ))
  }

  const renderConnections = () => {
    return connections.map(conn => {
      const sourceBlock = blocks.find(block => block.id === conn.source)
      const targetBlock = blocks.find(block => block.id === conn.target)
      
      if (!sourceBlock || !targetBlock) return null
      
      // Calculate connector positions (center of blocks)
      const sourceX = sourceBlock.position.x + 100
      const sourceY = sourceBlock.position.y + 50
      const targetX = targetBlock.position.x
      const targetY = targetBlock.position.y + 50
      
      return (
        <svg
          key={conn.id}
          className="absolute top-0 left-0 w-full h-full pointer-events-none"
        >
          <path
            d={`M ${sourceX} ${sourceY} C ${sourceX + 50} ${sourceY}, ${targetX - 50} ${targetY}, ${targetX} ${targetY}`}
            stroke="#4B5563"
            strokeWidth="2"
            fill="none"
            markerEnd="url(#arrowhead)"
          />
        </svg>
      )
    })
  }

  return (
    <div className="flex h-[calc(100vh-150px)]">
      {/* Left Sidebar - Building Blocks */}
      <div className="w-64 bg-white shadow-md flex flex-col">
        <div className="p-4 flex-1 overflow-y-auto">
          <div className="space-y-2">
            {(['agent', 'message', 'email', 'google'] as BlockType[]).map(blockType => (
              <div
                key={blockType}
                className="border rounded-md p-3 bg-white cursor-grab hover:bg-gray-50 flex items-center"
                draggable
                onDragStart={(e) => handleDragStart(e, blockType)}
              >
                <div className="text-gray-600 mr-2">
                  {getBlockIcon(blockType)}
                </div>
                <span className="text-gray-700">{getBlockTitle(blockType)}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="p-4 border-t">
          <button
            onClick={() => navigate('/flow-designer')}
            className="w-full px-4 py-2 text-gray-700 border rounded-md hover:bg-gray-50"
          >
            Back to Flow Designer
          </button>
        </div>
      </div>
      
      {/* Canvas Area */}
      <div
        className="flex-1 bg-gray-50 relative overflow-auto"
        onDragOver={handleDragOver}
        onDrop={handleDrop}
        onMouseMove={handleCanvasMouseMove}
        onMouseUp={handleCanvasMouseUp}
        ref={canvasRef}
      >
        <div className="absolute top-0 left-0 right-0 p-4 flex justify-between bg-white shadow-sm z-10">
          <h1 className="text-xl font-semibold text-gray-800">
            {templateName || 'New Workflow'}
          </h1>
          <button
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            Save Workflow
          </button>
        </div>
        
        {/* SVG Definitions */}
        <svg className="absolute" width="0" height="0">
          <defs>
            <marker
              id="arrowhead"
              markerWidth="10"
              markerHeight="7"
              refX="9"
              refY="3.5"
              orient="auto"
            >
              <polygon points="0 0, 10 3.5, 0 7" fill="#4B5563" />
            </marker>
          </defs>
        </svg>
        
        {/* Connection Lines */}
        {renderConnections()}
        
        {/* Temporary Connection Line */}
        {connectingFrom && (
          <div className="absolute top-0 left-0 w-full h-full pointer-events-none">
            <svg className="w-full h-full">
              <path
                d={`M ${blocks.find(b => b.id === connectingFrom)?.position.x ?? 0 + 100} ${blocks.find(b => b.id === connectingFrom)?.position.y ?? 0 + 50} L ${mousePosX} ${mousePosY}`}
                stroke="#4B5563"
                strokeWidth="2"
                strokeDasharray="5,5"
                fill="none"
              />
            </svg>
          </div>
        )}
        
        {/* Blocks */}
        {blocks.map(block => (
          <div
            key={block.id}
            className={`absolute w-[200px] bg-white border rounded-md shadow-sm
              ${selectedBlock === block.id ? 'ring-2 ring-blue-500' : ''}`}
            style={{
              left: `${block.position.x}px`,
              top: `${block.position.y}px`,
            }}
            onMouseDown={(e) => handleBlockMouseDown(e, block.id)}
            onMouseUp={() => handleBlockMouseUp(block.id)}
          >
            <div className="flex items-center justify-between p-3 border-b">
              <div className="flex items-center">
                <div className="mr-2 text-gray-600">
                  {getBlockIcon(block.type)}
                </div>
                <span className="font-medium text-gray-700">{block.title}</span>
              </div>
              <button
                className="text-gray-400 hover:text-red-500"
                onClick={() => handleDeleteBlock(block.id)}
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="p-3">
              <p className="text-sm text-gray-600">
                {block.type === 'agent' && 'Configure AI agent behavior'}
                {block.type === 'message' && 'Send a message to user'}
                {block.type === 'email' && 'Send an email notification'}
                {block.type === 'google' && 'Search for information'}
              </p>
            </div>
            
            {/* Connection Dots */}
            <div
              className="absolute w-4 h-4 right-0 top-1/2 transform translate-x-1/2 -translate-y-1/2 bg-blue-500 rounded-full cursor-pointer"
              onMouseDown={(e) => handleConnectorMouseDown(e, block.id)}
            />
            
            <div
              className="absolute w-4 h-4 left-0 top-1/2 transform -translate-x-1/2 -translate-y-1/2 bg-blue-500 rounded-full cursor-pointer"
              onMouseDown={(e) => handleConnectorMouseDown(e, block.id)}
            />
          </div>
        ))}
      </div>
    </div>
  )
}

export default FlowCanvas 