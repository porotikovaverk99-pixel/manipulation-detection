import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { SCORERS, getDashboardMetrics } from '../../services/api';
import { StateMessage } from '../../components/ui';
import styles from './Dashboard.module.css';

const COLORS = {
  high: '#ef4444',
  medium: '#f59e0b',
  low: '#10b981',
  accent: '#4e90f9',
  ink: '#0f172a',
  muted: '#64748b',
};

const EMPTY_METRICS = {
  summary: {
    total_cases: 0,
    mean_risk: 0,
    high_risk_share: 0,
    total_posts: 0,
    avg_posts_per_case: 0,
    open_cases: 0,
    closed_cases: 0,
    finalized_cases: 0,
    rumour_cases: 0,
    non_rumour_cases: 0,
  },
  engagement: {
    post_count: 0,
    total_likes: 0,
    total_reposts: 0,
    total_replies: 0,
    total_engagement: 0,
    unique_accounts: 0,
    verified_accounts: 0,
  },
  cases_over_time: [],
  risk_distribution: { high: 0, medium: 0, low: 0 },
  top_events: [],
  scorer_performance: [],
  timeline_data: [],
  manipulation_stats: null,
  status_breakdown: [],
  label_breakdown: [],
  source_breakdown: [],
  dataset_breakdown: [],
  top_risk_cases: [],
};

export function Dashboard() {
  const [metrics, setMetrics] = useState(null);
  const [globalMetrics, setGlobalMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [sourceName, setSourceName] = useState('all');
  const [scorerKey, setScorerKey] = useState('case_ensemble_v1');
  const [riskLevel, setRiskLevel] = useState('all');
  const [label, setLabel] = useState('all');
  const [status, setStatus] = useState('all');
  const [eventName, setEventName] = useState('all');

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([
      getDashboardMetrics({
        source_name: sourceName,
        scorer_key: scorerKey,
        risk_level: riskLevel,
        label,
        status,
        event_name: eventName,
      }),
      getDashboardMetrics({ scorer_key: scorerKey }),
    ])
      .then(([filteredData, globalData]) => {
        if (!cancelled) {
          setMetrics(filteredData);
          setGlobalMetrics(globalData);
        }
      })
      .catch((err) => {
        if (!cancelled) setError(err.message || 'Failed to load dashboard.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [sourceName, scorerKey, riskLevel, label, status, eventName]);

  const data = metrics || EMPTY_METRICS;
  const globalData = globalMetrics || data || EMPTY_METRICS;
  const summary = data.summary || EMPTY_METRICS.summary;
  const engagement = data.engagement || EMPTY_METRICS.engagement;
  const riskDistribution = data.risk_distribution || EMPTY_METRICS.risk_distribution;
  const casesOverTime = data.cases_over_time || [];
  const topEvents = data.top_events || [];
  const scorerPerformance = data.scorer_performance || [];
  const globalScorerPerformance = globalData.scorer_performance || [];
  const globalEngagement = globalData.engagement || EMPTY_METRICS.engagement;
  const globalDatasetBreakdown = globalData.dataset_breakdown || [];
  const globalSourceBreakdown = globalData.source_breakdown || [];
  const timelineData = data.timeline_data || [];
  const manipulationStats = data.manipulation_stats || null;
  const topRiskCases = data.top_risk_cases || [];
  const statusBreakdown = data.status_breakdown || [];
  const labelBreakdown = data.label_breakdown || [];
  const sourceBreakdown = data.source_breakdown || [];
  const datasetBreakdown = data.dataset_breakdown || [];
  const sourceOptions = ['all', ...Array.from(new Set(globalSourceBreakdown.map((item) => item.name))).sort()];

  const totalCases = summary.total_cases || riskDistribution.high + riskDistribution.medium + riskDistribution.low;
  const selectedScorer = SCORERS.find((scorer) => scorer.key === scorerKey);
  const highRiskShare = totalCases ? riskDistribution.high / totalCases : 0;
  const modelCoverage = totalCases
    ? Math.max(...scorerPerformance.map((item) => item.cases_analyzed || 0), 0) / totalCases
    : 0;
  const reviewBacklog = summary.open_cases + riskDistribution.high;
  const verifiedShare = engagement.unique_accounts ? engagement.verified_accounts / engagement.unique_accounts : 0;

  const riskPieData = [
    { name: 'High', value: riskDistribution.high, color: COLORS.high },
    { name: 'Medium', value: riskDistribution.medium, color: COLORS.medium },
    { name: 'Low', value: riskDistribution.low, color: COLORS.low },
  ].filter((item) => item.value > 0);

  const eventOptions = ['all', ...Array.from(new Set(topEvents.map((item) => item.event_name))).sort()];
  const statusOptions = ['all', ...Array.from(new Set(statusBreakdown.map((item) => item.name))).sort()];
  const labelOptions = ['all', ...Array.from(new Set(labelBreakdown.map((item) => item.name))).sort()];

  if (loading) return <StateMessage title="Loading dashboard" text="Building investigation overview." />;
  if (error) return <StateMessage title="Dashboard unavailable" text={error} />;

  return (
    <section className={`${styles.root} content dashboard dashboard-pro`}>
      <div className="page-head1">
        <h1>Analytics Dashboard</h1>
        <p>
          Operational view across cases, model signals, risk backlog, engagement, datasets, and events.
        </p>
      </div>

      <div className="dashboard-global-grid">
        {globalScorerPerformance.length > 0 && <ScorerCoverage items={globalScorerPerformance} totalCases={globalData.summary?.total_cases || totalCases} />}
        <EngagementPanel engagement={globalEngagement} fallbackPosts={globalData.summary?.total_posts || summary.total_posts} />
        {globalDatasetBreakdown.length > 0 && <BreakdownCard title="Datasets" items={globalDatasetBreakdown} compact />}
        {globalSourceBreakdown.length > 0 && <BreakdownCard title="Sources" items={globalSourceBreakdown} compact />}
      </div>

      <div className="dashboard-controls dashboard-filter-card">
        <label className="select-chip">
          <span>source</span>
          <select value={sourceName} onChange={(event) => setSourceName(event.target.value)}>
            {sourceOptions.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>scorer</span>
          <select value={scorerKey} onChange={(event) => setScorerKey(event.target.value)}>
            {SCORERS.map((scorer) => <option key={scorer.key} value={scorer.key}>{scorer.label}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>risk</span>
          <select value={riskLevel} onChange={(event) => setRiskLevel(event.target.value)}>
            {['all', 'high', 'medium', 'low'].map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>status</span>
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            {statusOptions.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>label</span>
          <select value={label} onChange={(event) => setLabel(event.target.value)}>
            {labelOptions.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip wide">
          <span>event</span>
          <select value={eventName} onChange={(event) => setEventName(event.target.value)}>
            {eventOptions.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <button className="button ghost" onClick={() => { setSourceName('all'); setScorerKey('case_ensemble_v1'); setRiskLevel('all'); setLabel('all'); setStatus('all'); setEventName('all'); }}>
          Clear
        </button>
      </div>

      <div className="dashboard-kpi-grid">
        <KpiCard label="Cases" value={formatNumber(totalCases)} hint={`${formatNumber(summary.total_posts)} posts`} />
        <KpiCard label="Mean risk" value={formatScore(summary.mean_risk)} hint={`${formatPercent(highRiskShare)} high-risk share`} tone={riskTone(summary.mean_risk)} />
        <KpiCard label="Engagement" value={formatNumber(engagement.total_engagement)} hint={`${formatNumber(engagement.unique_accounts)} unique accounts`} tone="accent" />
        <KpiCard label="Rumour / Non" value={`${formatNumber(summary.rumour_cases)} / ${formatNumber(summary.non_rumour_cases)}`} hint="label balance" />
        <KpiCard label="Verified authors" value={formatPercent(verifiedShare)} hint={`${formatNumber(engagement.verified_accounts)} verified accounts`} tone="low" />
        <KpiCard label="Post analysis" value={formatNumber(manipulationStats?.total_analyzed || 0)} hint={`avg ${formatScore(manipulationStats?.avg_manipulation_score || 0)}`} tone="accent" />
      </div>

      <div className="dashboard-layout">
        <div className="dashboard-main">
          <div className="chart-card dashboard-card large">
            <div className="panel-title">
              <h2>Case risk trend</h2>
              <span className="chart-subtitle">All available history, grouped by first event date</span>
            </div>
            <ResponsiveContainer width="100%" height={320}>
              <LineChart data={casesOverTime}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="date" fontSize={11} />
                <YAxis fontSize={11} />
                <Tooltip />
                <Legend />
                <Line type="monotone" dataKey="high_risk" stroke={COLORS.high} strokeWidth={2} dot={false} name="High" />
                <Line type="monotone" dataKey="medium_risk" stroke={COLORS.medium} strokeWidth={2} dot={false} name="Medium" />
                <Line type="monotone" dataKey="low_risk" stroke={COLORS.low} strokeWidth={2} dot={false} name="Low" />
                <Line type="monotone" dataKey="total" stroke={COLORS.ink} strokeWidth={2} dot={false} name="Total" />
              </LineChart>
            </ResponsiveContainer>
          </div>

          {(statusBreakdown.length > 0 || labelBreakdown.length > 0) && (
            <div className="dashboard-two-col">
              {statusBreakdown.length > 0 && <BreakdownCard title="Status" items={statusBreakdown} />}
              {labelBreakdown.length > 0 && <BreakdownCard title="Labels" items={labelBreakdown} />}
            </div>
          )}

          <div className="chart-card dashboard-card">
            <div className="chart-header">
              <h3>Top events</h3>
              <span className="chart-subtitle">Volume and average risk by event</span>
            </div>
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={topEvents.slice(0, 8)} layout="vertical" margin={{ left: 120 }}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis type="number" fontSize={11} />
                <YAxis type="category" dataKey="event_name" width={115} tick={{ fontSize: 11 }} />
                <Tooltip />
                <Bar dataKey="case_count" fill={COLORS.accent} radius={[0, 4, 4, 0]} name="Cases" />
              </BarChart>
            </ResponsiveContainer>
          </div>

        </div>

        <aside className="dashboard-side">
          <div className="chart-card dashboard-card compact">
            <div className="panel-title">
              <h2>Risk mix</h2>
              <span className="chart-subtitle">Selected scorer</span>
            </div>
            <ResponsiveContainer width="100%" height={220}>
              <PieChart>
                <Pie data={riskPieData} cx="50%" cy="50%" innerRadius={52} outerRadius={82} paddingAngle={3} dataKey="value">
                  {riskPieData.map((entry) => <Cell key={entry.name} fill={entry.color} />)}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
            <div className="risk-legend">
              <span><i className="dot high" />High {formatNumber(riskDistribution.high)}</span>
              <span><i className="dot med" />Medium {formatNumber(riskDistribution.medium)}</span>
              <span><i className="dot low" />Low {formatNumber(riskDistribution.low)}</span>
            </div>
          </div>

          {topRiskCases.length > 0 && <TopRiskCases cases={topRiskCases} />}
        </aside>
      </div>
    </section>
  );
}

function KpiCard({ label, value, hint, tone = 'neutral' }) {
  return (
    <div className={`dash-kpi ${tone}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{hint}</small>
    </div>
  );
}

function BreakdownCard({ title, items, compact = false }) {
  const max = Math.max(...items.map((item) => item.case_count || 0), 1);
  return (
    <div className={`chart-card dashboard-card breakdown-card ${compact ? 'compact' : ''}`}>
      <div className="panel-title">
        <h2>{title}</h2>
        <span className="chart-subtitle">cases · avg risk</span>
      </div>
      <div className="dash-breakdown-list">
        {items.length === 0 ? <p className="empty compact-empty">No data</p> : items.map((item) => (
          <div className="dash-breakdown-row" key={item.name}>
            <div>
              <span>{item.name || 'unknown'}</span>
              <small>{formatNumber(item.case_count)} cases · {formatScore(item.avg_risk)}</small>
            </div>
            <div className="dash-meter"><i style={{ width: `${Math.max(6, (item.case_count / max) * 100)}%` }} /></div>
          </div>
        ))}
      </div>
    </div>
  );
}

function TopRiskCases({ cases }) {
  return (
    <div className="chart-card dashboard-card compact">
      <div className="panel-title">
        <h2>Highest-risk cases</h2>
        <span className="chart-subtitle">Open these first</span>
      </div>
      <div className="top-risk-list">
        {cases.length === 0 ? <p className="empty compact-empty">No cases</p> : cases.map((item) => (
          <Link to={`/cases/${item.id}`} className="top-risk-item" key={item.id}>
            <div>
              <strong>{item.title}</strong>
              <small>#{item.id} · {item.event_name || item.external_case_id} · {item.post_count} posts</small>
            </div>
            <span className={`risk-score-pill ${riskClassName(item.risk_level)}`}>{formatScore(item.risk_score)}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}

function ScorerCoverage({ items, totalCases }) {
  const max = Math.max(totalCases, ...items.map((item) => item.cases_analyzed || 0), 1);
  return (
    <div className="chart-card dashboard-card compact">
      <div className="panel-title">
        <h2>Model coverage</h2>
        <span className="chart-subtitle">outputs by scorer</span>
      </div>
      <div className="scorer-coverage-list">
        {items.map((item) => (
          <div className="scorer-coverage-row" key={item.scorer_key}>
            <div>
              <strong>{scorerLabel(item.scorer_key)}</strong>
              <small>{formatNumber(item.cases_analyzed)} cases · avg {formatScore(item.avg_risk_score)}</small>
            </div>
            <div className="dash-meter"><i style={{ width: `${Math.max(4, (item.cases_analyzed / max) * 100)}%` }} /></div>
          </div>
        ))}
      </div>
    </div>
  );
}

function EngagementPanel({ engagement, fallbackPosts = 0 }) {
  return (
    <div className="chart-card dashboard-card compact">
      <div className="panel-title">
        <h2>Engagement signals</h2>
        <span className="chart-subtitle">posts in selected slice</span>
      </div>
      <div className="engagement-signal-grid">
        <span>▦ <strong>{formatNumber(engagement.post_count || fallbackPosts)}</strong><small>posts</small></span>
        <span>♥ <strong>{formatNumber(engagement.total_likes)}</strong><small>likes</small></span>
        <span>↻ <strong>{formatNumber(engagement.total_reposts)}</strong><small>reposts</small></span>
        <span>◉ <strong>{formatNumber(engagement.unique_accounts)}</strong><small>accounts</small></span>
      </div>
    </div>
  );
}

function scorerLabel(key) {
  return SCORERS.find((item) => item.key === key)?.label || key;
}

function riskTone(score) {
  if ((score || 0) >= 0.7) return 'high';
  if ((score || 0) >= 0.4) return 'medium';
  return 'low';
}

function riskClassName(level) {
  if (level === 'medium') return 'med';
  return level || 'low';
}

function formatNumber(value) {
  return new Intl.NumberFormat('en-US').format(Math.round(Number(value || 0)));
}

function formatScore(value) {
  const number = Number(value || 0);
  return number.toFixed(2);
}

function formatPercent(value) {
  return `${Math.round(Number(value || 0) * 100)}%`;
}
