/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
