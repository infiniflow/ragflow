import LineChart from '@/components/line-chart';
import { useTranslate } from '@/hooks/common-hooks';
import { IStats } from '@/interfaces/database/chat';
import { formatDate } from '@/utils/date';
import camelCase from 'lodash/camelCase';
import { useSelectChartStatsList } from '../hooks';

import styles from './index.module.less';

const StatsLineChart = ({ statsType }: { statsType: keyof IStats }) => {
  const { t } = useTranslate('chat');
  const chartList = useSelectChartStatsList();
  const list =
    chartList[statsType]?.map((x) => ({
      ...x,
      xAxis: formatDate(x.xAxis),
    })) ?? [];

  return (
    <div className={styles.chartItem}>
      <b className={styles.chartLabel}>{t(camelCase(statsType))}</b>
      <LineChart data={list}></LineChart>
    </div>
  );
};

const StatsChart = () => {
  return (
    <div className={styles.chartWrapper}>
      <StatsLineChart statsType={'pv'}></StatsLineChart>
      <StatsLineChart statsType={'round'}></StatsLineChart>
      <StatsLineChart statsType={'speed'}></StatsLineChart>
      <StatsLineChart statsType={'thumb_up'}></StatsLineChart>
      <StatsLineChart statsType={'tokens'}></StatsLineChart>
      <StatsLineChart statsType={'uv'}></StatsLineChart>
    </div>
  );
};

export default StatsChart;
