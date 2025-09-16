import { CustomTimeline, TimelineNode } from '@/components/originui/timeline';
import {
  CheckLine,
  FilePlayIcon,
  Grid3x2,
  ListPlus,
  PlayIcon,
} from 'lucide-react';
import { useMemo } from 'react';
export const TimelineNodeObj = {
  begin: {
    id: 1,
    title: 'Begin',
    icon: <PlayIcon size={13} />,
    clickable: false,
  },
  parser: { id: 2, title: 'Parser', icon: <FilePlayIcon size={13} /> },
  chunker: { id: 3, title: 'Chunker', icon: <Grid3x2 size={13} /> },
  indexer: {
    id: 4,
    title: 'Indexer',
    icon: <ListPlus size={13} />,
    clickable: false,
  },
  complete: {
    id: 5,
    title: 'Complete',
    icon: <CheckLine size={13} />,
    clickable: false,
  },
};

export interface TimelineDataFlowProps {
  activeId: number | string;
  activeFunc: (id: number | string) => void;
}
const TimelineDataFlow = ({ activeFunc, activeId }: TimelineDataFlowProps) => {
  // const [activeStep, setActiveStep] = useState(2);
  const timelineNodes: TimelineNode[] = useMemo(() => {
    const nodes: TimelineNode[] = [];
    Object.keys(TimelineNodeObj).forEach((key) => {
      nodes.push({
        ...TimelineNodeObj[key as keyof typeof TimelineNodeObj],
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
    activeFunc?.(id);
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
