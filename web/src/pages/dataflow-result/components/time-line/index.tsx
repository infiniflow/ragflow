import { CustomTimeline, TimelineNode } from '@/components/originui/timeline';
import {
  CheckLine,
  FilePlayIcon,
  Grid3x2,
  ListPlus,
  PlayIcon,
} from 'lucide-react';
import { useState } from 'react';

const TimelineDataFlow = () => {
  const [activeStep, setActiveStep] = useState(2);

  const timelineNodes: TimelineNode[] = [
    {
      id: 1,
      title: 'Begin',
      icon: <PlayIcon size={13} />,
      className: 'w-32',
      completed: false,
      clickable: false,
    },
    {
      id: 2,
      title: 'Parser',
      icon: <FilePlayIcon size={13} />,
      completed: false,
      className: 'w-32',
      content: '2m45s',
    },
    {
      id: 3,
      title: 'Chunker',
      icon: <Grid3x2 size={13} />,
      completed: false,
      className: 'w-32',
    },
    {
      id: 4,
      title: 'Indexer',
      className: 'w-32',
      icon: <ListPlus size={13} />,
      completed: false,
      clickable: false,
    },
    {
      id: 5,
      title: 'Complete',
      className: 'w-32',
      icon: <CheckLine size={13} />,
      completed: false,
      clickable: false,
    },
  ];

  const handleStepChange = (step: number, id: string | number) => {
    setActiveStep(step);
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
