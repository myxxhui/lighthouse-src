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
} from 'antd';
import { LoadingOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import CostTable from '@/components/CostTable';
import EfficiencyChart from '@/components/EfficiencyChart';
import ROITrendSection from '@/components/ROITrendSection';
import { useAppStore } from '@/store';
import type { CostTimeRange, CostCompareMode } from '@/types';
import type { DomainBreakdown } from '@/types';

const TIME_RANGE_OPTIONS: { label: string; value: CostTimeRange }[] = [
  { label: '近7天', value: '7d' },
  { label: '近30天', value: '30d' },
  { label: '本月', value: 'month' },
  { label: '本季度', value: 'quarter' },
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
    fetchGlobalCostMetrics,
    fetchNamespaceCosts,
    setUseMockData,
    setCostTimeRange,
    setCostCompareMode,
  } = useAppStore();

  useEffect(() => {
    fetchGlobalCostMetrics();
    fetchNamespaceCosts();
  }, [fetchGlobalCostMetrics, fetchNamespaceCosts, costTimeRange, costCompareMode]);

  const handleRowClick = (record: any) => {
    navigate(`/DrilldownPage?type=namespace&id=${record.namespace}`);
  };

  const renderGlobalMetrics = () => {
    if (loadingGlobalMetrics) {
      return (
        <Card loading={true}>
          <LoadingOutlined />
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
              prefix="¥"
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
            onClick={() => navigate('/DrilldownPage?focus=optimizable')}
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
              prefix="¥"
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
          {globalCostMetrics.domainBreakdown.map((domain, index) => (
            <Col key={index} xs={24} sm={12} md={8} lg={6}>
              <Card
                size="small"
                hoverable
                onClick={() => setDomainDetail(domain)}
                style={{ cursor: 'pointer' }}
              >
                <Statistic
                  title={domain.domain}
                  value={domain.cost}
                  prefix="¥"
                  formatter={value => Number(value).toLocaleString()}
                  suffix={
                    <div style={{ marginTop: 8 }}>
                      <small>效率: {domain.efficiency}%</small>
                    </div>
                  }
                />
              </Card>
            </Col>
          ))}
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
          <span>使用Mock数据</span>
          <Switch
            checked={useMockData}
            onChange={setUseMockData}
            checkedChildren="是"
            unCheckedChildren="否"
          />
        </Space>
      </div>

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
            value={costTimeRange}
            onChange={v => setCostTimeRange(v as CostTimeRange)}
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
                    navigate(`/DrilldownPage?type=namespace&id=${encodeURIComponent(ns.namespace)}`);
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
            <Descriptions.Item label="成本 (¥)">¥{domainDetail.cost.toLocaleString()}</Descriptions.Item>
            <Descriptions.Item label="可优化空间 (¥)">
              ¥{domainDetail.optimizableSpace.toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label="效率"> {domainDetail.efficiency}%</Descriptions.Item>
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
              总账单由基础计算、存储、网络及其它云产品费用汇总。下方为账单详情与领域分解。
            </p>
            {globalCostMetrics.billDetail && (
              <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
                <Descriptions.Item label="基础计算资源">¥{globalCostMetrics.billDetail.compute.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="存储">¥{globalCostMetrics.billDetail.storage.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="网络">¥{globalCostMetrics.billDetail.network.toLocaleString()}</Descriptions.Item>
                <Descriptions.Item label="其它云产品">¥{globalCostMetrics.billDetail.other.toLocaleString()}</Descriptions.Item>
              </Descriptions>
            )}
            <Descriptions column={1} bordered size="small">
              {globalCostMetrics.domainBreakdown.map((d, i) => (
                <Descriptions.Item key={i} label={d.domain}>
                  ¥{d.cost.toLocaleString()}（可优化空间 ¥{d.optimizableSpace.toLocaleString()}，效率 {d.efficiency}%）
                </Descriptions.Item>
              ))}
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
                ...globalCostMetrics.domainBreakdown.map(d => ({
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
                { title: '效率 (%)', dataIndex: 'efficiency', render: (v: number) => `${v}%` },
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
