import { useTranslate } from '@/hooks/common-hooks';
import { SettingOutlined } from '@ant-design/icons';
import { Button, Flex, Space, Typography } from 'antd';
import { ReactElement } from 'react';

const { Title, Paragraph } = Typography;

interface IProps {
  title: string;
  description: string;
  showRightButton?: boolean;
  rightButtonIcon?: ReactElement<any>;
  rightButtonTitle?: string;
  clickButton?: () => void;
}

const SettingTitle = ({
  title,
  description,
  clickButton,
  showRightButton = false,
  rightButtonIcon,
  rightButtonTitle,
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
            <Space>
              {rightButtonIcon || <SettingOutlined />}
              {rightButtonTitle || t('systemModelSettings')}
            </Space>
          </Flex>
        </Button>
      )}
    </Flex>
  );
};

export default SettingTitle;
