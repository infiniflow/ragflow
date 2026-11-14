import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { ReactNode } from 'react';

import { ViewMode } from './constants';
import { ViewModeNav } from './view-mode-nav';

export type CompilationViewProps = {
  viewMode: ViewMode;
  artifactsOpen: boolean;
  onArtifactsOpenChange: (open: boolean) => void;
  onViewModeChange: (mode: ViewMode) => void;
};

type CompilationViewLayoutProps = CompilationViewProps & {
  navigation?: ReactNode;
  children: ReactNode;
  defaultLeftSize?: number;
  showRightPanel?: boolean;
};

export function CompilationViewLayout({
  viewMode,
  artifactsOpen,
  onArtifactsOpenChange,
  onViewModeChange,
  navigation,
  children,
  defaultLeftSize = 33,
  showRightPanel = true,
}: CompilationViewLayoutProps) {
  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
      <ResizablePanelGroup direction="horizontal" className="flex-1">
        <ResizablePanel
          defaultSize={defaultLeftSize}
          minSize={20}
          maxSize={50}
          className="flex flex-col min-w-0"
        >
          <ViewModeNav
            viewMode={viewMode}
            open={artifactsOpen}
            onOpenChange={onArtifactsOpenChange}
            onViewModeChange={onViewModeChange}
          />
          {navigation && <div className="flex-1 min-h-0">{navigation}</div>}
        </ResizablePanel>
        {showRightPanel && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel className="flex flex-col min-w-0">
              {children}
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
    </Card>
  );
}
