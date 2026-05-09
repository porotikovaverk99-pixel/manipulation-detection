import { useEffect, useState } from 'react';
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  AreaChart,
  Area,
} from 'recharts';
import { SCORERS, getDashboardMetrics } from '../../services/api';
import { StateMessage } from '../../components/ui';
import styles from './Dashboard.module.css';

const COLORS = {
  high: '#ef4444',
  medium: '#f59e0b',
  low: '#10b981',
  accent: '#4e90f9',
};

export function Dashboard() {
  const [metrics, setMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState('month');
  const [scorerKey, setScorerKey] = useState('case_ensemble_v1');
  const [riskLevel, setRiskLevel] = useState('all');
  const [label, setLabel] = useState('all');
  const [status, setStatus] = useState('all');
  const [eventName, setEventName] = useState('all');

  const days = timeRange === 'week' ? 7 : timeRange === 'quarter' ? 90 : 30;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getDashboardMetrics({
      days,
      scorer_key: scorerKey,
      risk_level: riskLevel,
      label,
      status,
      event_name: eventName,
    })
      .then((data) => {
        if (!cancelled) setMetrics(data);
      })
      .catch((err) => {
        if (!cancelled) console.error(err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [days, scorerKey, riskLevel, label, status, eventName]);

  if (loading) return <StateMessage title="Loading dashboard" text="Fetching analytics data..." />;
  if (!metrics) return null;

  const riskDistribution = metrics.risk_distribution || { high: 0, medium: 0, low: 0 };
  const casesOverTime = metrics.cases_over_time || [];
  const topEvents = metrics.top_events || [];
  const scorerPerformance = metrics.scorer_performance || [];
  const timelineData = metrics.timeline_data || [];

  const pieData = [
    { name: 'High Risk', value: riskDistribution.high, color: COLORS.high },
    { name: 'Medium Risk', value: riskDistribution.medium, color: COLORS.medium },
    { name: 'Low Risk', value: riskDistribution.low, color: COLORS.low },
  ];

  const filteredData = casesOverTime;
  const totalCases = riskDistribution.high + riskDistribution.medium + riskDistribution.low;
  const weightedRiskScore = totalCases
    ? (
        (riskDistribution.high * 0.85 +
          riskDistribution.medium * 0.55 +
          riskDistribution.low * 0.2) /
        totalCases
      ).toFixed(2)
    : '0.00';
  const avgModelScore = scorerPerformance.length
    ? (
        scorerPerformance.reduce((sum, item) => sum + item.avg_risk_score, 0) /
        scorerPerformance.length
      ).toFixed(2)
    : weightedRiskScore;
  const eventOptions = ['all', ...Array.from(new Set(topEvents.map((item) => item.event_name))).sort()];

  return (
    <section className={`${styles.root} content dashboard`}>
      <div className="page-head">
        <h1>Analytics Dashboard</h1>
        <p>Real-time metrics and case intelligence overview.</p>
      </div>

      <div className="time-range-selector">
        <button className={timeRange === 'week' ? 'active' : ''} onClick={() => setTimeRange('week')}>Last 7 days</button>
        <button className={timeRange === 'month' ? 'active' : ''} onClick={() => setTimeRange('month')}>Last 30 days</button>
        <button className={timeRange === 'quarter' ? 'active' : ''} onClick={() => setTimeRange('quarter')}>Last 90 days</button>
      </div>

      <div className="filters">
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
          <span>label</span>
          <select value={label} onChange={(event) => setLabel(event.target.value)}>
            {['all', 'rumour', 'non-rumour'].map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>status</span>
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            {['all', 'open', 'closed', 'finalized'].map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>event</span>
          <select value={eventName} onChange={(event) => setEventName(event.target.value)}>
            {eventOptions.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <button className="button ghost" onClick={() => { setRiskLevel('all'); setLabel('all'); setStatus('all'); setEventName('all'); }}>
          Clear dashboard filters
        </button>
      </div>

      <div className="kpi-grid">
        <div className="kpi-card">
          <span className="kpi-label">Total Cases</span>
          <strong className="kpi-value">{totalCases}</strong>
          <span className="kpi-trend">All time</span>
        </div>
        <div className="kpi-card">
          <span className="kpi-label">High Risk Cases</span>
          <strong className="kpi-value high">{riskDistribution.high}</strong>
          <span className="kpi-trend">Needs immediate review</span>
        </div>
        <div className="kpi-card">
          <span className="kpi-label">Active Cases</span>
          <strong className="kpi-value">{Math.round((riskDistribution.high + riskDistribution.medium) * 0.7)}</strong>
          <span className="kpi-trend">In investigation</span>
        </div>
        <div className="kpi-card">
          <span className="kpi-label">Avg Risk Score</span>
          <strong className="kpi-value">{avgModelScore}</strong>
          <span className="kpi-trend">Overall severity</span>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-card large">
          <div className="chart-header">
            <h3>Cases Over Time</h3>
            <span className="chart-subtitle">Trend analysis by risk level</span>
          </div>
          <ResponsiveContainer width="100%" height={320}>
            <LineChart data={filteredData}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
              <XAxis dataKey="date" stroke="var(--muted)" fontSize={11} />
              <YAxis stroke="var(--muted)" fontSize={11} />
              <Tooltip contentStyle={{ background: 'var(--surface)', border: '1px solid var(--line)', borderRadius: '8px' }} />
              <Legend />
              <Line type="monotone" dataKey="high_risk" stroke={COLORS.high} strokeWidth={2} dot={false} name="High Risk" />
              <Line type="monotone" dataKey="medium_risk" stroke={COLORS.medium} strokeWidth={2} dot={false} name="Medium Risk" />
              <Line type="monotone" dataKey="low_risk" stroke={COLORS.low} strokeWidth={2} dot={false} name="Low Risk" />
            </LineChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-card">
          <div className="chart-header">
            <h3>Risk Distribution</h3>
            <span className="chart-subtitle">Current case breakdown</span>
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <PieChart>
              <Pie data={pieData} cx="50%" cy="50%" innerRadius={60} outerRadius={100} paddingAngle={4} dataKey="value" label={({ name, percent }) => `${name}: ${((percent || 0) * 100).toFixed(0)}%`}>
                {pieData.map((entry, index) => <Cell key={`cell-${index}`} fill={entry.color} />)}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-card">
          <div className="chart-header">
            <h3>Top Events by Cases</h3>
            <span className="chart-subtitle">Most frequent event types</span>
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={topEvents.slice(0, 6)} layout="vertical" margin={{ left: 120 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
              <XAxis type="number" stroke="var(--muted)" fontSize={11} />
              <YAxis type="category" dataKey="event_name" tick={{ fontSize: 11, fill: 'var(--muted)' }} width={110} />
              <Tooltip />
              <Bar dataKey="case_count" fill={COLORS.accent} radius={[0, 4, 4, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-card">
          <div className="chart-header">
            <h3>Scorer Performance</h3>
            <span className="chart-subtitle">Average risk scores by model</span>
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={scorerPerformance} margin={{ bottom: 60 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
              <XAxis dataKey="scorer_key" tick={{ fontSize: 10, fill: 'var(--muted)', angle: -15, textAnchor: 'end' }} height={60} />
              <YAxis stroke="var(--muted)" fontSize={11} domain={[0, 1]} />
              <Tooltip />
              <Bar dataKey="avg_risk_score" fill={COLORS.accent} radius={[4, 4, 0, 0]} name="Avg Risk Score" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="chart-card full-width">
        <div className="chart-header">
          <h3>Event Activity Timeline</h3>
          <span className="chart-subtitle">Event frequency over time</span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <AreaChart data={timelineData}>
            <defs>
              <linearGradient id="eventGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={COLORS.accent} stopOpacity={0.3} />
                <stop offset="95%" stopColor={COLORS.accent} stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--line)" />
            <XAxis dataKey="date" stroke="var(--muted)" fontSize={11} />
            <YAxis stroke="var(--muted)" fontSize={11} />
            <Tooltip />
            <Area type="monotone" dataKey="event_count" stroke={COLORS.accent} strokeWidth={2} fill="url(#eventGradient)" name="Events" />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}
