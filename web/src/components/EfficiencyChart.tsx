import React from 'react';
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts';

interface EfficiencyChartProps {
  efficiency: number;
  size?: number;
  showLabel?: boolean;
}

const EfficiencyChart: React.FC<EfficiencyChartProps> = ({
  efficiency,
  size = 120,
  showLabel = true,
}) => {
  const data = [
    { name: '效率', value: efficiency },
    { name: '可优化空间', value: 100 - efficiency },
  ];

  const COLORS = ['#52c41a', '#faad14'];

  const renderCustomizedLabel = ({ cx, cy, midAngle, innerRadius, outerRadius, percent }: any) => {
    const radius = innerRadius + (outerRadius - innerRadius) * 0.5;
    const x = cx + radius * Math.cos(-midAngle * (Math.PI / 180));
    const y = cy + radius * Math.sin(-midAngle * (Math.PI / 180));

    return (
      <text
        x={x}
        y={y}
        fill="white"
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={14}
        fontWeight="bold"
      >
        {`${(percent * 100).toFixed(0)}%`}
      </text>
    );
  };

  return (
    <div style={{ width: size, height: size }}>
      <ResponsiveContainer width="100%" height="100%">
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            innerRadius={size * 0.3}
            outerRadius={size * 0.5}
            paddingAngle={2}
            dataKey="value"
            labelLine={false}
            label={showLabel ? renderCustomizedLabel : undefined}
          >
            {data.map((entry, index) => (
              <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
            ))}
          </Pie>
          <Tooltip formatter={value => [`${value}%`, '']} />
        </PieChart>
      </ResponsiveContainer>
    </div>
  );
};

export default EfficiencyChart;
