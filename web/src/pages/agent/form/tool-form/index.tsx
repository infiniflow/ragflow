import useGraphStore from '../../store';
import { ToolFormConfigMap } from './constant';

const EmptyContent = () => <div></div>;

const ToolForm = () => {
  const clickedToolId = useGraphStore((state) => state.clickedToolId);

  const ToolForm =
    ToolFormConfigMap[clickedToolId as keyof typeof ToolFormConfigMap] ??
    EmptyContent;

  return (
    <section>
      <ToolForm key={clickedToolId}></ToolForm>
    </section>
  );
};

export default ToolForm;
