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

import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchKnowledgeBaseConfiguration,
  useFetchKnowledgeGraph,
} from '@/hooks/use-knowledge-request';
import { LucideFileText, SquarePen, Upload } from 'lucide-react';

enum ViewMode {
  Graph = 'graph',
  LlmWiki = 'llm-wiki',
}

const mockGraphData = {
  graph: {
    nodes: [
      {
        id: 'iPhone 17',
        entity_type: 'Product',
        communities: ['C1'],
        description: 'Next-generation AI-native smartphone',
        rank: 10,
        size: 120,
      },
      {
        id: 'A19 Pro',
        entity_type: 'Chip',
        communities: ['C1'],
        description: '3nm+ mobile processor',
        rank: 8,
        size: 80,
      },
      {
        id: 'Core Agent OS',
        entity_type: 'Software',
        communities: ['C2'],
        description: 'Agent-first operating system',
        rank: 7,
        size: 80,
      },
      {
        id: 'Photonic Sensor Array',
        entity_type: 'Hardware',
        communities: ['C2'],
        description: 'Next-gen camera sensor',
        rank: 6,
        size: 70,
      },
      {
        id: 'Quantum-Ready 6G',
        entity_type: 'Connectivity',
        communities: ['C1'],
        description: 'Future connectivity standard',
        rank: 5,
        size: 60,
      },
      {
        id: 'Neural Engine',
        entity_type: 'Component',
        communities: ['C2'],
        description: '15B parameter on-device inference',
        rank: 6,
        size: 70,
      },
    ],
    edges: [
      { source: 'iPhone 17', target: 'A19 Pro', weight: 5 },
      { source: 'iPhone 17', target: 'Core Agent OS', weight: 4 },
      { source: 'iPhone 17', target: 'Photonic Sensor Array', weight: 3 },
      { source: 'iPhone 17', target: 'Quantum-Ready 6G', weight: 2 },
      { source: 'A19 Pro', target: 'Neural Engine', weight: 4 },
      { source: 'Core Agent OS', target: 'Neural Engine', weight: 3 },
    ],
  },
};

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

  const graphData =
    knowledgeGraph?.graph && Object.keys(knowledgeGraph.graph).length > 0
      ? knowledgeGraph.graph
      : mockGraphData.graph;

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
        <div className="size-full flex items-center justify-center">
          <Empty
            type={EmptyType.Data}
            text={t('knowledgeDetails.graphPlaceholder')}
          />
        </div>
      ) : (
        <Card className="flex-1 min-h-0 overflow-hidden flex bg-bg-card border-border-button rounded-xl">
          <ResizablePanelGroup direction="horizontal">
            <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
              <WikiLeftPanel
                tab={leftTab}
                onTabChange={handleLeftTabChange}
                graphData={graphData}
              />
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
