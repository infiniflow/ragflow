import { useTranslate } from '@/hooks/common-hooks';
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
  const { t } = useTranslate('setting');

  return (
    <Flex align="center" justify={'space-between'}>
      <div>
        <Title level={5}>{title}</Title>
        <Paragraph>{description}</Paragraph>
      </div>
      {showRightButton && (
        <Button type={'primary'} onClick={clickButton}>
          <Flex align="center" gap={4}>
            <SettingOutlined />
            {t('systemModelSettings')}
          </Flex>
        </Button>
      )}
    </Flex>
  );
};

export default SettingTitle;
