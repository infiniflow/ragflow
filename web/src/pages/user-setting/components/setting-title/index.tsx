import { Typography } from 'antd';

const { Title, Paragraph } = Typography;

interface IProps {
  title: string;
  description: string;
}

const SettingTitle = ({ title, description }: IProps) => {
  return (
    <div>
      <Title level={5}>{title}</Title>
      <Paragraph>{description}</Paragraph>
    </div>
  );
};

export default SettingTitle;
