import React from 'react';
import { RadialBarChart, RadialBar, PolarAngleAxis, ResponsiveContainer } from 'recharts';

interface EfficiencyChartProps {
  efficiency: number;
  size?: number;
  showLabel?: boolean;
}

const EfficiencyChart: React.FC<EfficiencyChartProps> = ({ efficiency, size = 120, showLabel = true }) => {
  // Clamp efficiency to [0, 100]
  const value = Math.min(100, Math.max(0, efficiency));

  // Color: green > 60, amber 30-60, red < 30; gray when 0
  const getColor = (v: number) => {
    if (v <= 0) return '#6b7280';
    if (v >= 60) return '#10b981';
    if (v >= 30) return '#f59e0b';
    return '#ef4444';
  };
  const color = getColor(value);

  const data = [{ value, fill: color }];

  return (
    <div style={{ position: 'relative', width: size, height: size }}>
      <ResponsiveContainer width="100%" height="100%">
        <RadialBarChart
          cx="50%"
          cy="50%"
          innerRadius="68%"
          outerRadius="100%"
          startAngle={210}
          endAngle={-30}
          data={data}
          barSize={size * 0.1}
        >
          {/* Background track */}
          <PolarAngleAxis type="number" domain={[0, 100]} angleAxisId={0} tick={false} />
          <RadialBar
            background={{ fill: 'rgba(128,128,128,0.12)' }}
            dataKey="value"
            cornerRadius={size * 0.05}
            isAnimationActive
            animationDuration={800}
            animationEasing="ease-out"
          />
        </RadialBarChart>
      </ResponsiveContainer>
      {/* Center label */}
      {showLabel && (
        <div style={{
          position: 'absolute',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          textAlign: 'center',
          pointerEvents: 'none',
          marginTop: size * 0.06,
        }}>
          <div style={{
            fontSize: size * 0.2,
            fontWeight: 800,
            lineHeight: 1,
            color,
            letterSpacing: '-0.02em',
          }}>
            {value > 0 ? `${value}` : '—'}
          </div>
          {value > 0 && (
            <div style={{ fontSize: size * 0.1, color: 'rgba(128,128,128,0.7)', fontWeight: 500, marginTop: 2 }}>
              %
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default EfficiencyChart;
