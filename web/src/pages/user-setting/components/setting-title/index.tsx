import { Typography } from 'antd';

const { Title, Paragraph } = Typography;

const SettingTitle = () => {
  return (
    <div>
      <Title level={5}>设计资源</Title>
      <Paragraph>
        我们提供完善的设计原则、最佳实践和设计资源文件（来帮助业务快速设计出高质量的产品原型。
      </Paragraph>
    </div>
  );
};

export default SettingTitle;
