import { Operator, operatorIconMap } from './constant';

interface IProps {
  name: Operator;
  fontSize?: number;
  width?: number;
  color?: string;
}

const Empty = () => {
  return <div className="hidden"></div>;
};

const OperatorIcon = ({ name, fontSize, width, color }: IProps) => {
  const Icon = operatorIconMap[name] || Empty;
  return (
    <Icon
      className={'text-2xl max-h-6 max-w-6 text-[rgb(59, 118, 244)]'}
      style={{ fontSize, color }}
      width={width}
    ></Icon>
  );
};

export default OperatorIcon;
