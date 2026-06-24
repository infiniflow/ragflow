import BackButton from '@/components/back-button';
import NextMarkdownContent from '@/components/next-markdown-content';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { VersionHistorySheet } from '@/pages/dataset/compilation/version-history-sheet';
import {
  LeftPanelTab,
  WikiLeftPanel,
} from '@/pages/dataset/compilation/wiki-left-panel';
import { Routes } from '@/routes';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchKnowledgeBaseConfiguration,
  useFetchKnowledgeGraph,
} from '@/hooks/use-knowledge-request';
import KnowledgeForceGraph from '@/pages/dataset/compilation/knowledge-force-graph';
import { LucideFileText, SquarePen, Upload } from 'lucide-react';

enum ViewMode {
  Graph = 'graph',
  LlmWiki = 'llm-wiki',
}

const mockMarkdown = `# iPhone 17: The AI-Native Evolution

## Overview
The iPhone 17 represents a fundamental shift in mobile architecture, transitioning from a traditional smartphone to an **AI-Native Personal Agent**. Built upon the A19 Pro chip, it integrates deep-learning inference at the edge, ensuring your personal intelligence remains private and performant.

## Key Technical Specifications
| Feature | Specification | Impact |
|---|---|---|
| **Chipset** | A19 Pro (3nm+) | 30% faster NPU inference |
| **Display** | 6.9" ProMotion OLED | 3000 nits peak brightness |
| **Connectivity** | Quantum-Ready 6G | Ultra-low latency edge computing |
| **Intelligence** | Core Agent OS | Real-time on-device task automation |

## Core Innovations
### 1. The Core Agent OS
Unlike previous versions, iOS 19 on iPhone 17 is built around the **"Agent-First"** principle. The OS constantly observes usage patterns to proactively manage applications, background resources, and multi-modal communications.

> **Note:** The onboard Neural Engine now supports up to 15 billion parameters for local inference, meaning your data never leaves your device.

### 2. Photonics Camera System
The camera system has been upgraded to a **"Photonic Sensor Array"**, allowing for:
- **True-Depth Volumetric Capture:** Enabling 3D spatial video for immersive AR/VR environments.
- **Semantic Lighting Control:** AI dynamically adjusts lighting for every entity within a frame in real-time.

"meeting last Tuesday," and the system retrieves the`;

export default function Compilation() {
  const { t } = useTranslation();
  const { id } = useParams();
  const { navigateToDataFile } = useNavigatePage();
  const { data: knowledgeBase } = useFetchKnowledgeBaseConfiguration();
  const { data: knowledgeGraph } = useFetchKnowledgeGraph();
  const [leftTab, setLeftTab] = useState<LeftPanelTab>(LeftPanelTab.Graph);
  const [viewMode, setViewMode] = useState<ViewMode>(ViewMode.Graph);

  const handleSwitchToGraph = useCallback(() => {
    setViewMode(ViewMode.Graph);
  }, []);

  const handleSwitchToLlmWiki = useCallback(() => {
    setViewMode(ViewMode.LlmWiki);
  }, []);

  const handleLeftTabChange = useCallback((value: string) => {
    setLeftTab(value as LeftPanelTab);
  }, []);

  return (
    <section className="min-h-screen w-full flex flex-col p-4 gap-4 bg-bg-base">
      <header className="space-y-5">
        <BackButton
          to={`${Routes.DatasetBase}${Routes.Files}/${id}`}
          onClick={navigateToDataFile(id!)}
        >
          {t('common.back')}
        </BackButton>

        <section className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <RAGFlowAvatar
              avatar={knowledgeBase?.avatar}
              name={knowledgeBase?.name}
              className="size-10 rounded-lg"
            />
            <h2 className="text-xl font-medium text-text-primary">
              {knowledgeBase?.name}
              {t('knowledgeDetails.compilationTitleSuffix')}
            </h2>
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant={viewMode === ViewMode.Graph ? 'default' : 'outline'}
              size="sm"
              onClick={handleSwitchToGraph}
            >
              {t('knowledgeDetails.graph')}
            </Button>
            <Button
              variant={viewMode === ViewMode.LlmWiki ? 'default' : 'outline'}
              size="sm"
              onClick={handleSwitchToLlmWiki}
            >
              {t('knowledgeDetails.llmWiki')}
            </Button>
          </div>
        </section>
      </header>

      {viewMode === ViewMode.Graph ? (
        <div className="flex-1 min-h-0 flex flex-col">
          <KnowledgeForceGraph data={knowledgeGraph?.graph} show />
        </div>
      ) : (
        <Card className="flex-1 min-h-0 overflow-hidden flex bg-bg-card border-border-button rounded-xl">
          <ResizablePanelGroup direction="horizontal">
            <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
              <WikiLeftPanel tab={leftTab} onTabChange={handleLeftTabChange} />
            </ResizablePanel>
            <ResizableHandle withHandle />
            <ResizablePanel>
              <section className="size-full min-w-0 overflow-y-auto p-8">
                <div className="max-w-3xl mx-auto">
                  <div className="flex items-start justify-between mb-6">
                    <div className="flex items-center gap-3">
                      <h1 className="text-3xl font-semibold text-text-primary">
                        iPhone 17
                      </h1>
                      <span className="text-sm text-state-success bg-state-success/10 px-2 py-0.5 rounded">
                        test0415_2025_04_15_16_03
                      </span>
                    </div>

                    <div className="flex items-center gap-1">
                      <Button variant="ghost" size="icon" className="size-8">
                        <SquarePen className="size-4" />
                      </Button>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-8"
                          >
                            <Upload className="size-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>
                          {t('knowledgeDetails.export')}
                        </TooltipContent>
                      </Tooltip>
                      <VersionHistorySheet />
                    </div>
                  </div>

                  <NextMarkdownContent content={mockMarkdown} loading={false} />

                  <div className="mt-8 flex items-center gap-2 text-sm text-text-secondary">
                    <span className="flex items-center gap-1.5 px-2 py-1 bg-bg-base rounded">
                      <LucideFileText className="size-3.5" />
                      Mallat...17.pdf
                    </span>
                    <span className="px-2 py-1 bg-bg-base rounded">
                      #Company
                    </span>
                  </div>
                </div>
              </section>
            </ResizablePanel>
          </ResizablePanelGroup>
        </Card>
      )}
    </section>
  );
}
