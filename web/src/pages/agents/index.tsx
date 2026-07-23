import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { AgentCategory } from '@/constants/agent';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { AgentCanvasSection } from './agent-canvas-section';
import { CompilationOperatorSection } from './compilation-operator-section';
import { FlowType } from './constant';

const AgentTabList = [
  { value: FlowType.Flow, labelKey: 'tabList.ingestionPipeline' },
  { value: FlowType.Compiler, labelKey: 'tabList.compilationOperator' },
  { value: FlowType.Agent, labelKey: 'tabList.workflow' },
] as const;

export default function Agents() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab =
    (searchParams.get('tab') as FlowType) || FlowType.Flow;

  const handleTabChange = useCallback(
    (value: string) => {
      const next = new URLSearchParams(searchParams);
      next.set('tab', value);
      next.delete('page');
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const tabs = (
    <Tabs value={tab} onValueChange={handleTabChange}>
      <TabsList>
        {AgentTabList.map((x) => (
          <TabsTrigger
            key={x.value}
            value={x.value}
            data-testid={`agents-tab-${x.value}`}
          >
            {t(`flow.${x.labelKey}`)}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );

  if (tab === FlowType.Compiler) {
    return <CompilationOperatorSection tabs={tabs} />;
  }

  return (
    <AgentCanvasSection
      tabs={tabs}
      canvasCategory={
        tab === FlowType.Agent
          ? AgentCategory.AgentCanvas
          : AgentCategory.DataflowCanvas
      }
    />
  );
}
