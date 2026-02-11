import React from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import type { ROITrend } from '@/types';

interface TrendChartProps {
  data: ROITrend[];
  height?: number;
  showEfficiency?: boolean;
}

const TrendChart: React.FC<TrendChartProps> = ({ data, height = 300, showEfficiency = true }) => {
  const formatXAxis = (tickItem: string) => {
    const date = new Date(tickItem);
    return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' });
  };

  const CustomTooltip = ({ active, payload }: any) => {
    if (active && payload && payload.length) {
      const data = payload[0].payload;
      return (
        <div
          style={{
            backgroundColor: 'white',
            padding: '10px',
            border: '1px solid #ccc',
            borderRadius: '4px',
          }}
        >
          <p>
            <strong>日期: {data.date}</strong>
          </p>
          <p>价值: ¥{data.value.toLocaleString()}</p>
          <p>成本: ¥{data.cost.toLocaleString()}</p>
          {showEfficiency && <p>效率: {data.efficiency}%</p>}
        </div>
      );
    }
    return null;
  };

  return (
    <div style={{ width: '100%', height: height }}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis
            dataKey="date"
            tickFormatter={formatXAxis}
            angle={-45}
            textAnchor="end"
            height={60}
          />
          <YAxis yAxisId="left" tickFormatter={value => `¥${value.toLocaleString()}`} />
          {showEfficiency && (
            <YAxis
              yAxisId="right"
              orientation="right"
              domain={[0, 100]}
              tickFormatter={value => `${value}%`}
            />
          )}
          <Tooltip content={<CustomTooltip />} />
          <Legend />
          <Line
            yAxisId="left"
            type="monotone"
            dataKey="value"
            name="价值"
            stroke="#52c41a"
            strokeWidth={2}
            dot={{ r: 4 }}
            activeDot={{ r: 6 }}
          />
          <Line
            yAxisId="left"
            type="monotone"
            dataKey="cost"
            name="成本"
            stroke="#faad14"
            strokeWidth={2}
            dot={{ r: 4 }}
            activeDot={{ r: 6 }}
          />
          {showEfficiency && (
            <Line
              yAxisId="right"
              type="monotone"
              dataKey="efficiency"
              name="效率"
              stroke="#1890ff"
              strokeWidth={2}
              dot={{ r: 4 }}
              activeDot={{ r: 6 }}
            />
          )}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
};

export default TrendChart;
