import { SettingOutlined } from '@ant-design/icons';
import { Button, Flex, Typography } from 'antd';

const { Title, Paragraph } = Typography;

interface IProps {
  title: string;
  description: string;
  showRightButton?: boolean;
  clickButton?: () => void;
}

const SettingTitle = ({
  title,
  description,
  clickButton,
  showRightButton = false,
}: IProps) => {
  return (
    <Flex align="center" justify={'space-between'}>
      <div>
        <Title level={5}>{title}</Title>
        <Paragraph>{description}</Paragraph>
      </div>
      {showRightButton && (
        <Button type={'primary'} onClick={clickButton}>
          <SettingOutlined></SettingOutlined> System Model Settings
        </Button>
      )}
    </Flex>
  );
};

export default SettingTitle;
