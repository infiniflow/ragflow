import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Maximize2 } from 'lucide-react';
import {
  useRef,
  useState,
  type FC,
  type MouseEvent as ReactMouseEvent,
} from 'react';
import { useTranslation } from '../../hooks/use-translation';
import { cn } from '../../lib/utils';
import type { JSONSchema } from '../../types/json-schema';
import JsonSchemaVisualizer from './json-schema-visualizer';
import SchemaVisualEditor from './schema-visual-editor';

/** @public */
export interface JsonSchemaEditorProps {
  schema?: JSONSchema;
  setSchema?: (schema: JSONSchema) => void;
  className?: string;
}

/** @public */
const JsonSchemaEditor: FC<JsonSchemaEditorProps> = ({
  schema = { type: 'object' },
  setSchema,
  className,
}) => {
  // Handle schema changes and propagate to parent if needed
  const handleSchemaChange = (newSchema: JSONSchema) => {
    setSchema(newSchema);
  };

  const t = useTranslation();

  const [isFullscreen, setIsFullscreen] = useState(false);
  const [leftPanelWidth, setLeftPanelWidth] = useState(50); // percentage
  const resizeRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isDraggingRef = useRef(false);

  const toggleFullscreen = () => {
    setIsFullscreen(!isFullscreen);
  };

  const fullscreenClass = isFullscreen
    ? 'fixed inset-0 z-50 bg-background'
    : '';

  const handleMouseDown = (e: ReactMouseEvent) => {
    e.preventDefault();
    isDraggingRef.current = true;
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  const handleMouseMove = (e: MouseEvent) => {
    if (!isDraggingRef.current || !containerRef.current) return;

    const containerRect = containerRef.current.getBoundingClientRect();
    const newWidth =
      ((e.clientX - containerRect.left) / containerRect.width) * 100;

    // Limit the minimum and maximum width
    if (newWidth >= 20 && newWidth <= 80) {
      setLeftPanelWidth(newWidth);
    }
  };

  const handleMouseUp = () => {
    isDraggingRef.current = false;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
  };

  return (
    <div
      className={cn(
        'json-editor-container w-full',
        fullscreenClass,
        className,
        'jsonjoy',
      )}
    >
      {/* For mobile screens - show as tabs */}
      <div className="block lg:hidden w-full">
        <Tabs defaultValue="visual" className="w-full">
          <div className="flex items-center justify-between px-4 py-3 border-b w-full">
            <h3 className="font-medium">{t.schemaEditorTitle}</h3>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={toggleFullscreen}
                className="p-1.5 rounded-md hover:bg-secondary transition-colors"
                aria-label="Toggle fullscreen"
              >
                <Maximize2 size={16} />
              </button>
              <TabsList className="grid grid-cols-2 w-[200px]">
                <TabsTrigger value="visual">
                  {t.schemaEditorEditModeVisual}
                </TabsTrigger>
                <TabsTrigger value="json">
                  {t.schemaEditorEditModeJson}
                </TabsTrigger>
              </TabsList>
            </div>
          </div>

          <TabsContent
            value="visual"
            className={cn(
              'focus:outline-hidden w-full',
              isFullscreen ? 'h-screen' : 'h-[500px]',
            )}
          >
            <SchemaVisualEditor schema={schema} onChange={handleSchemaChange} />
          </TabsContent>

          <TabsContent
            value="json"
            className={cn(
              'focus:outline-hidden w-full',
              isFullscreen ? 'h-screen' : 'h-[500px]',
            )}
          >
            <JsonSchemaVisualizer
              schema={schema}
              onChange={handleSchemaChange}
            />
          </TabsContent>
        </Tabs>
      </div>

      {/* For large screens - show side by side */}
      <div
        ref={containerRef}
        className={cn(
          'hidden lg:flex lg:flex-col w-full',
          isFullscreen ? 'h-screen' : 'h-[600px]',
        )}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b w-full shrink-0">
          <h3 className="font-medium">{t.schemaEditorTitle}</h3>
          <button
            type="button"
            onClick={toggleFullscreen}
            className="p-1.5 rounded-md hover:bg-secondary transition-colors"
            aria-label={t.schemaEditorToggleFullscreen}
          >
            <Maximize2 size={16} />
          </button>
        </div>
        <div className="flex flex-row w-full grow min-h-0">
          <div
            className="h-full min-h-0"
            style={{ width: `${leftPanelWidth}%` }}
          >
            <SchemaVisualEditor schema={schema} onChange={handleSchemaChange} />
          </div>
          {/** biome-ignore lint/a11y/noStaticElementInteractions: What exactly does this div do? */}
          <div
            ref={resizeRef}
            className="w-1 bg-border hover:bg-primary cursor-col-resize shrink-0"
            onMouseDown={handleMouseDown}
          />
          <div
            className="h-full min-h-0"
            style={{ width: `${100 - leftPanelWidth}%` }}
          >
            <JsonSchemaVisualizer
              schema={schema}
              onChange={handleSchemaChange}
            />
          </div>
        </div>
      </div>
    </div>
  );
};

export default JsonSchemaEditor;
