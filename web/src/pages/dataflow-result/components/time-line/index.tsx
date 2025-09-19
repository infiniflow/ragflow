import { CustomTimeline, TimelineNode } from '@/components/originui/timeline';
import {
  CheckLine,
  FilePlayIcon,
  Grid3x2,
  ListPlus,
  PlayIcon,
} from 'lucide-react';
import { useMemo } from 'react';
export enum TimelineNodeType {
  begin = 'begin',
  parser = 'parser',
  chunk = 'chunk',
  indexer = 'indexer',
  complete = 'complete',
  end = 'end',
}
export const TimelineNodeArr = [
  {
    id: 1,
    title: 'Begin',
    icon: <PlayIcon size={13} />,
    clickable: false,
    type: TimelineNodeType.begin,
  },
  {
    id: 2,
    title: 'Parser',
    icon: <FilePlayIcon size={13} />,
    type: TimelineNodeType.parser,
  },
  {
    id: 3,
    title: 'Chunker',
    icon: <Grid3x2 size={13} />,
    type: TimelineNodeType.chunk,
  },
  {
    id: 4,
    title: 'Indexer',
    icon: <ListPlus size={13} />,
    clickable: false,
    type: TimelineNodeType.indexer,
  },
  {
    id: 5,
    title: 'Complete',
    icon: <CheckLine size={13} />,
    clickable: false,
    type: TimelineNodeType.complete,
  },
];

export interface TimelineDataFlowProps {
  activeId: number | string;
  activeFunc: (id: number | string, step: TimelineNode) => void;
}
const TimelineDataFlow = ({ activeFunc, activeId }: TimelineDataFlowProps) => {
  // const [activeStep, setActiveStep] = useState(2);
  const timelineNodes: TimelineNode[] = useMemo(() => {
    const nodes: TimelineNode[] = [];
    TimelineNodeArr.forEach((node) => {
      nodes.push({
        ...node,
        className: 'w-32',
        completed: false,
      });
    });
    return nodes;
  }, []);

  const activeStep = useMemo(() => {
    const index = timelineNodes.findIndex((node) => node.id === activeId);
    return index > -1 ? index + 1 : 0;
  }, [activeId, timelineNodes]);
  const handleStepChange = (step: number, id: string | number) => {
    // setActiveStep(step);
    activeFunc?.(
      id,
      timelineNodes.find((node) => node.id === activeStep) as TimelineNode,
    );
    console.log(step, id);
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
          nodeSize={24}
          activeStyle={{
            nodeSize: 30,
            iconColor: 'var(--accent-primary)',
            textColor: 'var(--accent-primary)',
          }}
        />
      </div>
    </div>
  );
};

export default TimelineDataFlow;
