import { CustomTimeline, TimelineNode } from '@/components/originui/timeline';
import {
  Blocks,
  File,
  FilePlay,
  FileStack,
  Heading,
  ListPlus,
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
    icon: <File size={13} />,
    clickable: false,
  },
  [TimelineNodeType.parser]: {
    title: 'Parser',
    icon: <FilePlay size={13} />,
  },
  [TimelineNodeType.contextGenerator]: {
    title: 'Context Generator',
    icon: <FileStack size={13} />,
  },
  [TimelineNodeType.titleSplitter]: {
    title: 'Title Splitter',
    icon: <Heading size={13} />,
  },
  [TimelineNodeType.characterSplitter]: {
    title: 'Title Splitter',
    icon: <Heading size={13} />,
  },
  [TimelineNodeType.splitter]: {
    title: 'Character Splitter',
    icon: <Blocks size={13} />,
  },
  [TimelineNodeType.tokenizer]: {
    title: 'Tokenizer',
    icon: <ListPlus size={13} />,
    clickable: false,
  },
};
// export const TimelineNodeArr = [
//   {
//     id: 1,
//     title: 'File',
//     icon: <PlayIcon size={13} />,
//     clickable: false,
//     type: TimelineNodeType.begin,
//   },
//   {
//     id: 2,
//     title: 'Context Generator',
//     icon: <PlayIcon size={13} />,
//     type: TimelineNodeType.contextGenerator,
//   },
//   {
//     id: 3,
//     title: 'Title Splitter',
//     icon: <PlayIcon size={13} />,
//     type: TimelineNodeType.titleSplitter,
//   },
//   {
//     id: 4,
//     title: 'Character Splitter',
//     icon: <PlayIcon size={13} />,
//     type: TimelineNodeType.characterSplitter,
//   },
//   {
//     id: 5,
//     title: 'Tokenizer',
//     icon: <CheckLine size={13} />,
//     clickable: false,
//     type: TimelineNodeType.tokenizer,
//   },
// ]
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
