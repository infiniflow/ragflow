import * as echarts from 'echarts/core';
import {
  GridComponent,
  GridComponentOption,
  LegendComponent,
  LegendComponentOption,
  TitleComponent,
  TitleComponentOption,
  TooltipComponent,
  TooltipComponentOption,
} from 'echarts/components';
import {
  BarChart,
  BarSeriesOption,
  LineChart,
  LineSeriesOption,
  PieChart,
  PieSeriesOption,
} from 'echarts/charts';
import { LabelLayout, UniversalTransition } from 'echarts/features';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  PieChart,
  LineChart,
  CanvasRenderer,
  LabelLayout,
  UniversalTransition,
  BarChart,
]);

export type EChartsOption = echarts.ComposeOption<
  TooltipComponentOption | TitleComponentOption | LegendComponentOption | PieSeriesOption | LineSeriesOption | GridComponentOption | BarSeriesOption
>;

export default echarts;
