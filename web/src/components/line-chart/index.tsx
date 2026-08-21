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

import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { CategoricalChartProps } from 'recharts/types/chart/generateCategoricalChart';

interface IProps extends CategoricalChartProps {
  data?: Array<{ xAxis: string; yAxis: number }>;
  showLegend?: boolean;
}

const RagLineChart = ({ data, showLegend = false }: IProps) => {
  return (
    <ResponsiveContainer width="100%" height="100%">
      <LineChart
        // width={500}
        // height={300}
        data={data}
        margin={{
          // top: 5,
          // right: 30,
          // left: 20,
          // bottom: 10,
        }}
      >
        <CartesianGrid strokeDasharray="3 3" />
        <XAxis dataKey="xAxis" />
        <YAxis />
        <Tooltip />
        {showLegend && <Legend />}
        <Line
          type="monotone"
          dataKey="yAxis"
          stroke="#8884d8"
          activeDot={{ r: 8 }}
        />
        {/* <Line type="monotone" dataKey="uv" stroke="#82ca9d" /> */}
      </LineChart>
    </ResponsiveContainer>
  );
};

export default RagLineChart;
