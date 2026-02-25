import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'umi';
import {
  Card,
  Row,
  Col,
  Statistic,
  Switch,
  Space,
  Alert,
  Tooltip,
  Segmented,
  Select,
  Modal,
  Descriptions,
  Table,
  Tabs,
  DatePicker,
} from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { LoadingOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import CostTable from '@/components/CostTable';
import EfficiencyChart from '@/components/EfficiencyChart';
import ROITrendSection from '@/components/ROITrendSection';
import { useAppStore } from '@/store';
import type { CostTimeRange, CostCompareMode } from '@/types';
import type { DomainBreakdown } from '@/types';
import { CURRENCY_SYMBOL } from '@/constants';

const TIME_RANGE_OPTIONS: { label: string; value: CostTimeRange }[] = [
  { label: '昨天', value: '1d' },
  { label: '近7天', value: '7d' },
  { label: '近30天', value: '30d' },
  { label: '本月', value: 'month' },
  { label: '本季度', value: 'quarter' },
  { label: '自定义', value: 'custom' },
];

const COMPARE_OPTIONS: { label: string; value: CostCompareMode }[] = [
  { label: '不对比', value: 'none' },
  { label: '对比上一周期', value: 'previous' },
];

type DetailModalType = 'bill' | 'efficiency' | null;

const CostOverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const tabFromUrl = searchParams.get('tab');
  const activeTab = tabFromUrl === 'roi' ? 'roi' : 'cost';
  const [detailModal, setDetailModal] = useState<DetailModalType>(null);
  const [domainDetail, setDomainDetail] = useState<DomainBreakdown | null>(null);
  const {
    globalCostMetrics,
    namespaceCosts,
    loadingGlobalMetrics,
    loadingNamespaceCosts,
    errorGlobalMetrics,
    errorNamespaceCosts,
    useMockData,
    costTimeRange,
    costCompareMode,
    costCustomDateRange,
    selectedDimension,
    fetchGlobalCostMetrics,
    fetchNamespaceCosts,
    setUseMockData,
    setCostTimeRange,
    setCostCompareMode,
    setCostCustomDateRange,
  } = useAppStore();

  useEffect(() => {
    fetchGlobalCostMetrics();
    fetchNamespaceCosts();
  }, [fetchGlobalCostMetrics, fetchNamespaceCosts, costTimeRange, costCompareMode, costCustomDateRange]);

  const handleRowClick = (record: any) => {
    navigate(
      `/DrilldownPage?dimension=${selectedDimension}&type=namespace&id=${encodeURIComponent(record.namespace)}`,
    );
  };

  const renderGlobalMetrics = () => {
    if (loadingGlobalMetrics) {
      const isDateRange = costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1];
      const daysHint =
        isDateRange && costCustomDateRange
          ? Math.max(0, Math.ceil((new Date(costCustomDateRange[1]).getTime() - new Date(costCustomDateRange[0]).getTime()) / 86400000) + 1)
          : 0;
      return (
        <Card loading={true}>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }}>
            <LoadingOutlined spin style={{ fontSize: 24 }} />
            <span>{isDateRange ? '正在汇总所选日期成本…' : '加载中...'}</span>
            {isDateRange && daysHint > 0 && (
              <span style={{ fontSize: 12, color: '#666' }}>
                已选 {daysHint} 天，可能需要 10～30 秒
              </span>
            )}
          </div>
        </Card>
      );
    }

    if (errorGlobalMetrics) {
      return (
        <Alert message="加载全局指标失败" description={errorGlobalMetrics} type="error" showIcon />
      );
    }

    if (!globalCostMetrics) {
      return <Card>暂无数据</Card>;
    }

    const prev = globalCostMetrics.previousPeriod;
    // 周期对比算法：环比 = (本期 - 上期) / 上期 * 100；效率为百分点差
    const costChange =
      prev && prev.totalBillableCost > 0
        ? ((globalCostMetrics.totalBillableCost - prev.totalBillableCost) / prev.totalBillableCost) * 100
        : null;
    const optimChange =
      prev && prev.totalOptimizableSpace > 0
        ? ((globalCostMetrics.totalOptimizableSpace - prev.totalOptimizableSpace) /
            prev.totalOptimizableSpace) *
          100
        : null;
    const effChange = prev ? globalCostMetrics.globalEfficiency - prev.globalEfficiency : null;

    return (
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card hoverable onClick={() => setDetailModal('bill')} style={{ cursor: 'pointer' }}>
            <Statistic
              title={
                <Space>
                  总账单成本
                  <Tooltip title="各资源类型（计算、存储、网络、其它云产品）计费成本汇总。明细见下方领域成本分解与命名空间成本明细。">
                    <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
                  </Tooltip>
                </Space>
              }
              value={globalCostMetrics.totalBillableCost}
              prefix={CURRENCY_SYMBOL}
              formatter={value => Number(value).toLocaleString()}
              suffix={
                costChange != null && (
                  <span
                    style={{
                      fontSize: 12,
                      marginLeft: 4,
                      color: costChange >= 0 ? '#ff4d4f' : '#52c41a',
                    }}
                  >
                    {costChange >= 0 ? '+' : ''}
                    {costChange.toFixed(1)}% 较上期
                  </span>
                )
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card
            hoverable
            onClick={() => navigate(`/DrilldownPage?dimension=${selectedDimension}&focus=optimizable`)}
            style={{ cursor: 'pointer' }}
          >
            <Statistic
              title={
                <Space>
                  可优化空间
                  <Tooltip title="各资源类型可优化空间汇总（账单成本减使用成本）。点击可下钻至命名空间、服务组、Pod 查看明细。">
                    <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
                  </Tooltip>
                </Space>
              }
              value={globalCostMetrics.totalOptimizableSpace}
              prefix={CURRENCY_SYMBOL}
              formatter={value => Number(value).toLocaleString()}
              suffix={
                optimChange != null && (
                  <span
                    style={{
                      fontSize: 12,
                      marginLeft: 4,
                      color: optimChange >= 0 ? '#ff4d4f' : '#52c41a',
                    }}
                  >
                    {optimChange >= 0 ? '+' : ''}
                    {optimChange.toFixed(1)}% 较上期
                  </span>
                )
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card hoverable onClick={() => setDetailModal('efficiency')} style={{ cursor: 'pointer' }}>
            <Statistic
              title={
                <Space>
                  全局效率分
                  <Tooltip title="汇总使用成本/汇总账单成本×100%。各层级效率构成见领域与命名空间明细。">
                    <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
                  </Tooltip>
                </Space>
              }
              value={globalCostMetrics.globalEfficiency}
              suffix={
                <>
                  %
                  {effChange != null && (
                    <span
                      style={{
                        fontSize: 12,
                        marginLeft: 4,
                        color: effChange >= 0 ? '#52c41a' : '#ff4d4f',
                      }}
                    >
                      {effChange >= 0 ? '+' : ''}
                      {effChange.toFixed(1)}% 较上期
                    </span>
                  )}
                </>
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <div
              style={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                height: '100%',
              }}
            >
              <EfficiencyChart efficiency={globalCostMetrics.globalEfficiency} size={80} />
            </div>
          </Card>
        </Col>
      </Row>
    );
  };

  const renderDomainBreakdown = () => {
    if (!globalCostMetrics?.domainBreakdown?.length) {
      return null;
    }

    return (
      <Card title="领域成本分解" style={{ marginTop: 16 }}>
        <Row gutter={[16, 16]}>
          {globalCostMetrics.domainBreakdown.map((domain, index) => {
            const isStorageOrNetwork = domain.domain === '存储' || domain.domain === '网络';
            const card = (
              <Card
                size="small"
                hoverable
                onClick={() => setDomainDetail(domain)}
                style={{ cursor: 'pointer' }}
              >
                <Statistic
                  title={
                    isStorageOrNetwork ? (
                      <Tooltip title="当前仅展示成本金额，深度分析后续开放">
                        <span>{domain.domain} <QuestionCircleOutlined style={{ fontSize: 12, color: '#999' }} /></span>
                      </Tooltip>
                    ) : (
                      domain.domain
                    )
                  }
                  value={domain.cost}
                  prefix={CURRENCY_SYMBOL}
                  formatter={value => Number(value).toLocaleString()}
                  suffix={
                    <div style={{ marginTop: 8 }}>
                      <small>效率: {domain.efficiency}%</small>
                    </div>
                  }
                />
              </Card>
            );
            return (
              <Col key={index} xs={24} sm={12} md={8} lg={6}>
                {card}
              </Col>
            );
          })}
        </Row>
      </Card>
    );
  };

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16,
        }}
      >
        <h2>全域成本透视</h2>
        <Space>
          <span style={{ color: '#666', fontSize: 12 }}>
            {useMockData ? '数据来源：Mock' : (globalCostMetrics && globalCostMetrics.totalBillableCost > 0 ? '数据来源：云账单' : '数据来源：—')}
            {globalCostMetrics?.lastUpdatedAt && (
              <> · 数据更新至 {new Date(globalCostMetrics.lastUpdatedAt).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })}</>
            )}
          </span>
          <span>使用Mock数据</span>
          <Switch
            checked={useMockData}
            onChange={setUseMockData}
            checkedChildren="是"
            unCheckedChildren="否"
          />
        </Space>
      </div>

      {!useMockData && (
        <Alert
          message="当前为账期汇总数据（真实数据）"
          description="为什么所有时间线数据一样？后端当前仅返回整账期汇总（月/季度），不按天切分，所以切换时间范围（昨天/7天/30天/本月/本季度）不会改变结果。对比上一周期：账期汇总模式下暂无上期数据，仅 Mock 数据可展示环比。如需按不同时间范围或对比看到差异，可开启「使用Mock数据」。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}
      {!useMockData &&
        globalCostMetrics &&
        globalCostMetrics.totalBillableCost === 0 &&
        (!namespaceCosts || namespaceCosts.length === 0) && (
          <Alert
            message="暂无真实数据"
            description="当前未接入真实数据源。请完成：1) 01_ 成本透视真实数据：配置阿里云 AK/SK 并执行 ETL，将云账单写入 cost_cloud_bill_summary；2) 02_ 真实数据源：接入 Prometheus/K8s 获取集群内计算成本。完成后关闭 Mock 即可看到真实成本与领域占比。"
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}

      <Alert
        message="当前版本能力范围"
        description="已上线：全域成本透视、成本钻取、SLO 红绿灯、ROI 看板。智能预防、智能故障处理即将推出。"
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Card style={{ marginBottom: 16 }}>
        <Space wrap size="middle">
          <span>时间范围：</span>
          <Segmented
            value={costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1] ? 'custom' : costTimeRange}
            onChange={v => {
              const val = v as CostTimeRange;
              if (val === 'custom') {
                setCostTimeRange('custom');
                setCostCustomDateRange(null);
              } else {
                setCostCustomDateRange(null);
                setCostTimeRange(val);
              }
            }}
            options={TIME_RANGE_OPTIONS}
          />
          <span style={{ marginLeft: 8 }}>对比：</span>
          <Select
            value={costCompareMode}
            onChange={v => setCostCompareMode(v as CostCompareMode)}
            options={COMPARE_OPTIONS}
            style={{ width: 140 }}
          />
        </Space>
        {!useMockData && (costTimeRange === 'custom' || (costCustomDateRange != null && costCustomDateRange[0] && costCustomDateRange[1])) && (
          <div style={{ marginTop: 12 }}>
            <span style={{ marginRight: 8 }}>自定义日期（最多 6 个月内）：</span>
            <DatePicker.RangePicker
              value={costCustomDateRange?.[0] && costCustomDateRange?.[1] ? [dayjs(costCustomDateRange[0]), dayjs(costCustomDateRange[1])] : null}
              onChange={(dates: [Dayjs | null, Dayjs | null] | null) => {
                if (dates?.[0] && dates?.[1]) {
                  const from = dates[0].format('YYYY-MM-DD');
                  const to = dates[1].format('YYYY-MM-DD');
                  if (dates[1].diff(dates[0], 'day') <= 180) {
                    setCostTimeRange('custom');
                    setCostCustomDateRange([from, to]);
                  }
                } else {
                  setCostCustomDateRange(null);
                }
              }}
            />
          </div>
        )}
      </Card>

      {/* 四环境卡片 POC/FAT/UAT/PROD [Ref: 01_设计 §按环境展示 D9-4] */}
      <Card title="按环境总账" style={{ marginBottom: 16 }}>
        <Row gutter={[16, 16]}>
          {(['POC', 'FAT', 'UAT', 'PROD'] as const).map(env => {
            const item = globalCostMetrics?.envBreakdown?.find(e => e.environment === env);
            const isConfigured = item && (item.account_id || item.total_cost > 0 || item.account_display_name !== '未配置');
            const displayName = item?.account_display_name ?? '未配置';
            const totalCost = item?.total_cost ?? 0;
            const changePct = item?.change_pct;
            const card = (
              <Col key={env} xs={24} sm={12} md={6}>
                <Card
                  size="small"
                  title={env}
                  hoverable={!!isConfigured}
                  onClick={isConfigured ? () => navigate(`/CostDrilldownEnvPage?env=${env}`) : undefined}
                  style={{ cursor: isConfigured ? 'pointer' : 'default' }}
                >
                  <Statistic
                    title={displayName}
                    value={totalCost}
                    prefix={CURRENCY_SYMBOL}
                    formatter={value => Number(value).toLocaleString()}
                    suffix={
                      changePct != null && !Number.isNaN(changePct) ? (
                        <span style={{ fontSize: 12, color: changePct >= 0 ? '#ff4d4f' : '#52c41a' }}>
                          {changePct >= 0 ? '+' : ''}{changePct.toFixed(1)}% 较上期
                        </span>
                      ) : null
                    }
                  />
                </Card>
              </Col>
            );
            return card;
          })}
        </Row>
      </Card>

      <Tabs
        activeKey={activeTab}
        onChange={key => {
          setSearchParams(key === 'roi' ? { tab: 'roi' } : {});
        }}
        items={[
          {
            key: 'cost',
            label: '成本结构',
            children: (
              <>
                {renderGlobalMetrics()}
                {renderDomainBreakdown()}
                <Card title="命名空间成本明细" style={{ marginTop: 16 }}>
                  {loadingNamespaceCosts ? (
                    <div style={{ textAlign: 'center', padding: '20px' }}>
                      <LoadingOutlined spin /> 加载中...
                    </div>
                  ) : errorNamespaceCosts ? (
                    <Alert
                      message="加载命名空间成本失败"
                      description={errorNamespaceCosts}
                      type="error"
                      showIcon
                    />
                  ) : namespaceCosts?.length ? (
                    <CostTable data={namespaceCosts} onRowClick={handleRowClick} />
                  ) : (
                    <div style={{ textAlign: 'center', padding: '20px' }}>暂无命名空间数据</div>
                  )}
                </Card>
              </>
            ),
          },
          {
            key: 'roi',
            label: 'ROI 价值追踪',
            children: <ROITrendSection />,
          },
        ]}
      />

      <Modal
        title={domainDetail ? `${domainDetail.domain} 详情` : undefined}
        open={domainDetail !== null}
        onCancel={() => setDomainDetail(null)}
        footer={
          domainDetail ? (
            <div>
              <span style={{ marginRight: 8 }}>从命名空间下钻至服务组、Pod：</span>
              {(namespaceCosts || []).slice(0, 3).map(ns => (
                <button
                  key={ns.namespace}
                  type="button"
                  onClick={() => {
                    setDomainDetail(null);
                    navigate(
                      `/DrilldownPage?dimension=${selectedDimension}&type=namespace&id=${encodeURIComponent(ns.namespace)}`,
                    );
                  }}
                  style={{ marginRight: 8 }}
                >
                  {ns.namespace}
                </button>
              ))}
            </div>
          ) : null
        }
        width={520}
      >
        {domainDetail && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="领域">{domainDetail.domain}</Descriptions.Item>
            <Descriptions.Item label={`成本 (${CURRENCY_SYMBOL})`}>{CURRENCY_SYMBOL}{domainDetail.cost.toLocaleString()}</Descriptions.Item>
            <Descriptions.Item label={`可优化空间 (${CURRENCY_SYMBOL})`}>
              {domainDetail.optimizableSpace != null && domainDetail.optimizableSpace > 0
                ? `${CURRENCY_SYMBOL}${domainDetail.optimizableSpace.toLocaleString()}`
                : '—'}
            </Descriptions.Item>
            <Descriptions.Item label="效率">
              {domainDetail.efficiency != null && domainDetail.efficiency > 0 ? `${domainDetail.efficiency}%` : '—'}
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>

      <Modal
        title={detailModal === 'bill' ? '成本账单详情' : '效率构成'}
        open={detailModal !== null}
        onCancel={() => setDetailModal(null)}
        footer={null}
        width={640}
      >
        {detailModal === 'bill' && globalCostMetrics && (
          <div>
            <p style={{ color: '#666', marginBottom: 16 }}>
              总账单由计算资源、存储、网络、其它四类领域汇总，分项之和=总账单。可优化空间与效率在无数据支撑时显示「—」。下方按领域列出该账期成本最高的产品（最多 4 个），金额为该领域下该产品的总成本（非单价）。点击「查看详情」可跳转钻取页（账期汇总数据下无命名空间级下钻，仅 Mock 时可下钻）。
            </p>
            {globalCostMetrics.billDetail && (
              <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
                <Descriptions.Item label="基础计算资源">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.compute.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="存储">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.storage.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="网络">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.network.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="其它云产品">{CURRENCY_SYMBOL}{globalCostMetrics.billDetail.other.toLocaleString()}</Descriptions.Item>
              </Descriptions>
            )}
            <Descriptions column={1} bordered size="small">
              {(globalCostMetrics.domainBreakdown ?? []).map((d, i) => {
                const dimensionAnchor = (d.domain === '计算资源' && 'compute') || (d.domain === '存储' && 'storage') || (d.domain === '网络' && 'network') || null;
                return (
                  <Descriptions.Item key={i} label={d.domain}>
                    <div>
                      <div>
                        {CURRENCY_SYMBOL}{d.cost.toLocaleString()}
                        （可优化空间 {d.optimizableSpace > 0 ? `${CURRENCY_SYMBOL}${d.optimizableSpace.toLocaleString()}` : '—'}，效率 {d.efficiency > 0 ? `${d.efficiency}%` : '—'}）
                      </div>
                      {d.topProducts && d.topProducts.length > 0 && (
                        <div style={{ marginTop: 8 }}>
                          <div style={{ marginBottom: 4, fontSize: 12, color: '#666' }}>成本前 4 产品（该领域该账期总成本，非单价）：</div>
                          <ul style={{ margin: 0, paddingLeft: 18 }}>
                            {(d.topProducts || []).map((p, j) => (
                              <li key={j}>{p.product}：{CURRENCY_SYMBOL}{p.cost.toLocaleString()}</li>
                            ))}
                          </ul>
                          {dimensionAnchor && (
                            <a
                              href={`#/DrilldownPage?dimension=${dimensionAnchor}`}
                              onClick={(e) => { e.preventDefault(); navigate(`/DrilldownPage?dimension=${dimensionAnchor}`); setDetailModal(null); }}
                              style={{ marginTop: 6, display: 'inline-block' }}
                            >
                              查看详情 →
                            </a>
                          )}
                        </div>
                      )}
                    </div>
                  </Descriptions.Item>
                );
              })}
            </Descriptions>
          </div>
        )}
        {detailModal === 'efficiency' && globalCostMetrics && (
          <div>
            <p style={{ color: '#666', marginBottom: 16 }}>
              全局效率 = 汇总使用成本/汇总账单成本×100%。各领域与命名空间效率如下。
            </p>
            <Table
              size="small"
              dataSource={[
                ...(globalCostMetrics.domainBreakdown ?? []).map(d => ({
                  key: `domain-${d.domain}`,
                  name: d.domain,
                  efficiency: d.efficiency,
                  type: '领域',
                })),
                ...(namespaceCosts || []).map(n => ({
                  key: `ns-${n.namespace}`,
                  name: n.namespace,
                  efficiency: n.efficiency,
                  type: '命名空间',
                })),
              ]}
              columns={[
                { title: '类型', dataIndex: 'type', width: 80 },
                { title: '名称', dataIndex: 'name' },
                { title: '效率 (%)', dataIndex: 'efficiency', render: (v: number) => (v != null && v > 0 ? `${v}%` : '—') },
              ]}
              pagination={false}
            />
          </div>
        )}
      </Modal>
    </div>
  );
};

export default CostOverviewPage;
