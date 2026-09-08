import { CustomTimeline, TimelineNode } from '@/components/originui/timeline';
import { useMemo } from 'react';
import { IPipelineFileLogDetail } from '../../interface';

export interface TimelineDataFlowProps {
  activeId: number | string;
  activeFunc: (id: number | string, step: TimelineNode) => void;
  data: IPipelineFileLogDetail;
  timelineNodes: TimelineNode[];
}
const TimelineDataFlow = ({
  activeFunc,
  activeId,
  timelineNodes,
}: TimelineDataFlowProps) => {
  // const [timelineNodeArr,setTimelineNodeArr] = useState<ITimelineNodeObj & {id: number | string}>()

  const activeStep = useMemo(() => {
    const index = timelineNodes.findIndex((node) => node.id === activeId);
    return index > -1 ? index + 1 : 0;
  }, [activeId, timelineNodes]);
  const handleStepChange = (step: number, id: string | number) => {
    activeFunc?.(
      id,
      timelineNodes.find((node) => node.id === activeStep) as TimelineNode,
    );
  };

  return (
    <div className="">
      <div>
        <CustomTimeline
          nodes={timelineNodes as TimelineNode[]}
          activeStep={activeStep}
          onStepChange={handleStepChange}
          orientation="horizontal"
          lineStyle="solid"
          lineColor="rgb(var(--))"
          nodeSize={24}
          activeStyle={{
            nodeSize: 30,
            iconColor: 'rgb(var(--accent-primary))',
            textColor: 'rgb(var(--accent-primary))',
          }}
        />
      </div>
    </div>
  );
};

export default TimelineDataFlow;
