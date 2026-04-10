/**
 * 与后端 billingCalendarRef 一致：业务日 YYYY-MM 用 Asia/Shanghai（北京时间；菲律宾等为 UTC+8 同日）。[Ref: 用户需求]
 */
export function billingCalendarPartsFromNow(): {
  y: number;
  m: number;
  monthStr: string;
  prevMonthStr: string;
  quarterKey: string;
  prevQuarterKey: string;
  yearStr: string;
  prevYearStr: string;
} {
  const cnDateStr = new Date().toLocaleDateString('sv-SE', { timeZone: 'Asia/Shanghai' });
  const [y, m] = cnDateStr.split('-').map(Number);
  const monthStr = `${y}-${String(m).padStart(2, '0')}`;
  const prevMonthStr = m === 1 ? `${y - 1}-12` : `${y}-${String(m - 1).padStart(2, '0')}`;
  const q = Math.floor((m - 1) / 3) + 1;
  const quarterKey = `${y}-Q${q}`;
  const prevQ = q <= 1 ? 4 : q - 1;
  const prevY = q <= 1 ? y - 1 : y;
  const prevQuarterKey = `${prevY}-Q${prevQ}`;
  return {
    y,
    m,
    monthStr,
    prevMonthStr,
    quarterKey,
    prevQuarterKey,
    yearStr: String(y),
    prevYearStr: String(y - 1),
  };
}
