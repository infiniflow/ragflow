# base-chart
基于[echarts](https://echarts.apache.org/zh/index.html)@5.X

## Props
| 参数 | 说明 | 类型 | 默认值 |
| --- | --- | --- | --- |
| option | 对应echarts的option,数据更新触发渲染 | Object | null |

## Event
| 事件 | 参数 | 说明 |
| --- | --- | --- |
| setOption | (option) | 手动调用触发更新渲染，比Props性能更优|