import { Typography } from 'antd';

import { useTranslate } from '@/hooks/common-hooks';
import styles from './index.less';

const { Title, Text } = Typography;

const LoginRightPanel = () => {
  const { t } = useTranslate('login');
  return <section className={styles.rightPanel}></section>;
};

export default LoginRightPanel;
