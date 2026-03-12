import { CustomTimeline, TimelineNode } from '@/components/originui/timeline';
import {
  LucideBlocks,
  LucideFile,
  LucideFilePlay,
  LucideFileStack,
  LucideHeading,
  LucideListPlus,
} from 'lucide-react';
import { useMemo } from 'react';
import { TimelineNodeType } from '../../constant';
import { IPipelineFileLogDetail } from '../../interface';

export type ITimelineNodeObj = {
  title: string;
  icon: JSX.Element;
  clickable?: boolean;
  type: TimelineNodeType;
};

export const TimelineNodeObj = {
  [TimelineNodeType.begin]: {
    title: 'File',
    icon: <LucideFile className="size-[1em]" />,
    clickable: false,
  },
  [TimelineNodeType.parser]: {
    title: 'Parser',
    icon: <LucideFilePlay className="size-[1em]" />,
  },
  [TimelineNodeType.contextGenerator]: {
    title: 'Context Generator',
    icon: <LucideFileStack className="size-[1em]" />,
  },
  [TimelineNodeType.titleSplitter]: {
    title: 'Title Splitter',
    icon: <LucideHeading className="size-[1em]" />,
  },
  [TimelineNodeType.characterSplitter]: {
    title: 'Character Splitter',
    icon: <LucideBlocks className="size-[1em]" />,
  },
  [TimelineNodeType.tokenizer]: {
    title: 'Tokenizer',
    icon: <LucideListPlus className="size-[1em]" />,
    clickable: false,
  },
};
export interface TimelineDataFlowProps {
  activeId: number | string;
  activeFunc: (id: number | string, step: TimelineNode) => void;
  data: IPipelineFileLogDetail;
  timelineNodes: TimelineNode[];
}
const TimelineDataFlow = ({
  activeFunc,
  activeId,
  data,
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
