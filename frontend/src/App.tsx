import React, { useEffect, useMemo, useState } from 'react';
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import './App.css';
import {
  SCORERS,
  getCaseDetails,
  getCaseScores,
  getCases,
  getModelComparison,
  recordDecision,
} from './services/api';
import {
  AnalystDecision,
  CaseDetails as CaseDetailsType,
  CaseListItem,
  CaseModelScore,
  CasesListResponse,
  ModelComparisonResponse,
  ModelComparisonRow,
  RiskLevel,
  ScorerKey,
} from './types';

const DEFAULT_SCORER: ScorerKey = 'case_ensemble_v1';

function App() {
  const [scorerKey, setScorerKey] = useState<ScorerKey>(DEFAULT_SCORER);

  return (
    <BrowserRouter>
      <div className="app-shell">
        <Sidebar />
        <main className="main">
          <Topbar scorerKey={scorerKey} setScorerKey={setScorerKey} />
          <Routes>
            <Route path="/" element={<Navigate to="/cases" replace />} />
            <Route path="/cases" element={<CasesQueue scorerKey={scorerKey} />} />
            <Route
              path="/cases/:id"
              element={<CaseDetails scorerKey={scorerKey} setScorerKey={setScorerKey} />}
            />
            <Route path="/evaluation" element={<ModelComparison />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}

function Sidebar() {
  const location = useLocation();
  const items = [
    { to: '/cases', label: 'Cases queue', icon: 'queue' },
    { to: '/evaluation', label: 'Model comparison', icon: 'chart' },
  ];

  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark" />
        <div>
          <div className="brand-name">case-detect</div>
          <div className="brand-sub">case-level analyst panel</div>
        </div>
      </div>

      <div className="nav-section">
        <div className="nav-label">Workspace</div>
        {items.map((item) => (
          <Link
            key={item.to}
            className={`nav-item ${location.pathname.startsWith(item.to) ? 'active' : ''}`}
            to={item.to}
          >
            <Icon name={item.icon} />
            <span>{item.label}</span>
          </Link>
        ))}
      </div>

      <div className="nav-section">
        <div className="nav-label">Next stage</div>
        <div className="nav-item disabled">
          <Icon name="users" />
          <span>Accounts</span>
          <span className="nav-count">soon</span>
        </div>
        <div className="nav-item disabled">
          <Icon name="live" />
          <span>Live ingest</span>
          <span className="nav-count">soon</span>
        </div>
      </div>

      <div className="sidebar-footer">
        <span className="dot-live" />
        <span>API · localhost:8080</span>
      </div>
    </aside>
  );
}

function Topbar({
  scorerKey,
  setScorerKey,
}: {
  scorerKey: ScorerKey;
  setScorerKey: (value: ScorerKey) => void;
}) {
  const location = useLocation();
  const crumb = location.pathname.startsWith('/evaluation') ? 'Model comparison' : 'Cases';

  return (
    <header className="topbar">
      <div className="crumbs">
        <span>Workspace</span>
        <span className="sep">/</span>
        <span className="current">{crumb}</span>
      </div>
      <div className="topbar-spacer" />
      <label className="select-chip">
        <span>scorer</span>
        <select value={scorerKey} onChange={(event) => setScorerKey(event.target.value as ScorerKey)}>
          {SCORERS.map((scorer) => (
            <option key={scorer.key} value={scorer.key}>
              {scorer.label}
            </option>
          ))}
        </select>
      </label>
    </header>
  );
}

function CasesQueue({ scorerKey }: { scorerKey: ScorerKey }) {
  const navigate = useNavigate();
  const [data, setData] = useState<CasesListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState({
    risk: 'all',
    event: 'all',
    label: 'all',
    status: 'all',
  });

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    getCases({
      scorer_key: scorerKey,
      source_name: 'pheme_large',
      dataset_name: 'pheme',
      dataset_split: 'eventcv_large',
      limit: 100,
    })
      .then((response) => {
        if (!cancelled) {
          setData(response);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [scorerKey]);

  const cases = useMemo(() => data?.items || [], [data]);
  const events = useMemo(() => Array.from(new Set(cases.map((item) => item.event_name))).sort(), [cases]);
  const filtered = useMemo(
    () =>
      cases
        .filter((item) => filters.risk === 'all' || item.risk_level === filters.risk)
        .filter((item) => filters.event === 'all' || item.event_name === filters.event)
        .filter((item) => filters.label === 'all' || item.label === filters.label)
        .filter((item) => filters.status === 'all' || item.status === filters.status),
    [cases, filters]
  );
  const summary = summarize(filtered);

  if (loading) {
    return <StateMessage title="Loading cases" text="Reading case-level scores from the backend." />;
  }
  if (error) {
    return <StateMessage title="Backend unavailable" text={error} />;
  }

  return (
    <section className="content">
      <div className="page-head">
        <h1>Cases queue</h1>
        <p>
          Ranked suspicious cases from <span className="mono">pheme / eventcv_large</span>. The active scorer is{' '}
          <span className="mono accent">{scorerKey}</span>.
        </p>
      </div>

      <div className="tiles">
        <SummaryTile label="Cases" value={summary.total} note={`${cases.length} loaded`} />
        <SummaryTile label="High risk" value={summary.high} note="score >= 0.70" tone="high" />
        <SummaryTile label="Medium risk" value={summary.medium} note="0.40 to 0.70" tone="medium" />
        <SummaryTile label="Low risk" value={summary.low} note="score < 0.40" tone="low" />
        <SummaryTile label="Mean risk" value={formatScore(summary.mean)} note="filtered queue" />
      </div>

      <div className="filters">
        <SelectFilter
          label="event"
          value={filters.event}
          onChange={(value) => setFilters((current) => ({ ...current, event: value }))}
          options={['all', ...events]}
        />
        <SelectFilter
          label="risk"
          value={filters.risk}
          onChange={(value) => setFilters((current) => ({ ...current, risk: value }))}
          options={['all', 'high', 'medium', 'low']}
        />
        <SelectFilter
          label="label"
          value={filters.label}
          onChange={(value) => setFilters((current) => ({ ...current, label: value }))}
          options={['all', 'rumour', 'non-rumour']}
        />
        <SelectFilter
          label="status"
          value={filters.status}
          onChange={(value) => setFilters((current) => ({ ...current, status: value }))}
          options={['all', 'open', 'closed', 'finalized', 'new', 'in_review', 'decided']}
        />
        <button className="button ghost" onClick={() => setFilters({ risk: 'all', event: 'all', label: 'all', status: 'all' })}>
          Clear
        </button>
      </div>

      <div className="cases-table">
        <div className="case-row table-head">
          <div>case</div>
          <div>title · event</div>
          <div>posts · duration</div>
          <div>score</div>
          <div>risk</div>
          <div>components</div>
          <div>evidence</div>
          <div>activity</div>
        </div>
        {filtered.map((item) => (
          <button key={item.id} className="case-row data-row" onClick={() => navigate(`/cases/${item.id}`)}>
            <div className="case-id">#{item.id}</div>
            <div className="case-title">
              <span>{item.title}</span>
              <small>
                {item.event_name} · {item.dataset_name}/{item.dataset_split}
              </small>
            </div>
            <div className="muted">
              <div>{item.post_count} posts</div>
              <div className="mono">{item.duration_min ? `${item.duration_min}m` : 'no range'}</div>
            </div>
            <ScoreBar value={item.risk_score} level={item.risk_level} />
            <RiskPill level={item.risk_level} />
            <ComponentMini scores={item.scores} />
            <div className="evidence-tags">
              {item.evidence.slice(0, 2).map((evidence) => (
                <span key={evidence.key}>{evidence.key}</span>
              ))}
            </div>
            <Sparkline data={item.timeline} />
          </button>
        ))}
      </div>
    </section>
  );
}

function CaseDetails({
  scorerKey,
  setScorerKey,
}: {
  scorerKey: ScorerKey;
  setScorerKey: (value: ScorerKey) => void;
}) {
  const params = useParams();
  const navigate = useNavigate();
  const id = Number(params.id);
  const [details, setDetails] = useState<CaseDetailsType | null>(null);
  const [scores, setScores] = useState<CaseModelScore[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [decision, setDecision] = useState<AnalystDecision | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([getCaseDetails(id, scorerKey), getCaseScores(id)])
      .then(([caseDetails, caseScores]) => {
        if (!cancelled) {
          setDetails(caseDetails);
          setScores(caseScores.items);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [id, scorerKey]);

  if (loading) {
    return <StateMessage title="Loading case" text={`Reading case #${id}.`} />;
  }
  if (error || !details) {
    return <StateMessage title="Case unavailable" text={error || 'Case not found.'} />;
  }

  const activeScore = scores.find((score) => score.scorer_key === scorerKey);

  const submitDecision = (value: AnalystDecision) => {
    setDecision(value);
    recordDecision(details.id, value).catch(() => undefined);
  };

  return (
    <section className="content">
      <button className="button ghost back-button" onClick={() => navigate('/cases')}>
        <Icon name="back" />
        Back to queue
      </button>

      <div className="page-head detail-head">
        <div>
          <div className="tags-line">
            <span className="case-id">#{details.id}</span>
            <span className="soft-tag">{details.external_id}</span>
            <span className="soft-tag">{details.status}</span>
            {details.label && <span className="soft-tag">{details.label}</span>}
          </div>
          <h1>{details.title}</h1>
          <p>
            Event <span className="mono">{details.event_name}</span> · source{' '}
            <span className="mono">{details.source_name}</span> · dataset{' '}
            <span className="mono">
              {details.dataset_name}/{details.dataset_split}
            </span>
          </p>
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-main">
          <section className="panel score-panel">
            <div>
              <PanelTitle title="Score breakdown" />
              <Breakdown label="Temporal" value={details.scores.temporal_score} />
              <Breakdown label="Coordination" value={details.scores.coordination_score} />
              <Breakdown label="Content" value={details.scores.content_score} />
              <Breakdown label="Bot contribution" value={details.scores.bot_score} />
            </div>
            <div className={`risk-display ${riskClass(details.risk_level)}`}>
              <span>Active risk score</span>
              <strong>{formatScore(activeScore?.risk_score ?? details.risk_score)}</strong>
              <RiskPill level={activeScore?.risk_level || details.risk_level} />
              <small>
                {details.post_count} posts · {details.duration_min ? `${details.duration_min} min` : 'no time range'}
              </small>
            </div>
          </section>

          <section className="panel">
            <PanelTitle title="Top evidence" subtitle="Feature values from persisted case_features" />
            <div className="evidence-list">
              {details.evidence.length > 0 ? (
                details.evidence.map((item) => <EvidenceRow key={item.key} item={item} />)
              ) : (
                <p className="empty">No feature evidence stored for this case.</p>
              )}
            </div>
          </section>

          <section className="panel timeline-panel">
            <PanelTitle title="Timeline" subtitle="Post density over the case duration" />
            <Timeline data={details.timeline} />
          </section>

          <section className="panel">
            <PanelTitle title="Thread" subtitle={`${details.posts.length} loaded posts`} />
            <div className="thread">
              {details.posts.slice(0, 30).map((post) => (
                <article key={post.id} className="post">
                  <div className="post-time">+{formatOffset(post.t_offset_sec)}</div>
                  <div>
                    <div className="post-head">
                      <span>@{post.author_handle}</span>
                      <span>{post.kind}</span>
                    </div>
                    <p>{post.text}</p>
                  </div>
                </article>
              ))}
            </div>
          </section>

          <section className="panel">
            <PanelTitle title="Accounts involved" />
            <div className="accounts-table">
              {details.accounts.map((account) => (
                <div key={account.id} className="account-row">
                  <div>
                    <strong>@{account.handle}</strong>
                    <small>joined {account.joined_month}</small>
                  </div>
                  <span className="mono">{account.posts} posts</span>
                  <span className="mono">{formatPercent(account.share)}</span>
                  <ScoreBar value={account.bot_score} level={scoreToLevel(account.bot_score)} compact />
                </div>
              ))}
            </div>
          </section>

          <section className="panel">
            <PanelTitle title="Shared artifacts" />
            <div className="artifact-grid">
              <ArtifactColumn title="URLs" items={details.artifacts.urls.map((item) => [item.url, item.count])} />
              <ArtifactColumn title="Hashtags" items={details.artifacts.hashtags.map((item) => [item.tag, item.count])} />
              <ArtifactColumn title="Phrases" items={details.artifacts.phrases.map((item) => [item.phrase, item.count])} />
            </div>
          </section>
        </div>

        <aside className="right-rail">
          <section className="panel">
            <PanelTitle title="Analyst decision" />
            <div className="decision-actions">
              <button onClick={() => submitDecision('suspicious')}>Mark suspicious</button>
              <button onClick={() => submitDecision('not_suspicious')}>Not suspicious</button>
              <button onClick={() => submitDecision('unclear')}>Unclear</button>
            </div>
            {decision && <p className="decision-note">Decision recorded as {decision}.</p>}
          </section>

          <section className="panel">
            <PanelTitle title="All scorers" />
            <div className="scorer-list">
              {scores.map((score) => (
                <button
                  key={score.scorer_key}
                  className={score.scorer_key === scorerKey ? 'active' : ''}
                  onClick={() => setScorerKey(score.scorer_key as ScorerKey)}
                >
                  <span>
                    {scoreLabel(score.scorer_key)}
                    <small>{score.scorer_key}</small>
                  </span>
                  <strong>{formatScore(score.risk_score)}</strong>
                </button>
              ))}
            </div>
          </section>

          <section className="panel">
            <PanelTitle title="Case metadata" />
            <dl className="metadata">
              <div>
                <dt>case_id</dt>
                <dd>{details.id}</dd>
              </div>
              <div>
                <dt>external_case_id</dt>
                <dd>{details.external_id}</dd>
              </div>
              <div>
                <dt>first_event_at</dt>
                <dd>{formatDate(details.first_event_at)}</dd>
              </div>
              <div>
                <dt>last_event_at</dt>
                <dd>{formatDate(details.last_event_at)}</dd>
              </div>
            </dl>
          </section>
        </aside>
      </div>
    </section>
  );
}

function ModelComparison() {
  const [data, setData] = useState<ModelComparisonResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getModelComparison()
      .then((response) => {
        if (!cancelled) {
          setData(response);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return <StateMessage title="Loading metrics" text="Reading model comparison from backend." />;
  }
  if (error || !data) {
    return <StateMessage title="Evaluation unavailable" text={error || 'No evaluation data.'} />;
  }

  const bestPr = bestBy(data.rows, 'pr_auc');
  const bestRoc = bestBy(data.rows, 'roc_auc');

  return (
    <section className="content">
      <div className="page-head">
        <h1>Model comparison</h1>
        <p>
          Runtime comparison from <span className="mono">case_model_scores</span> for{' '}
          <span className="mono">
            {data.dataset_name}/{data.dataset_split}
          </span>
          .
        </p>
      </div>

      <div className="tiles eval-tiles">
        <SummaryTile label="Eval cases" value={data.case_count} note={`${data.positive_cases} positive`} />
        <SummaryTile label="Best ranking model" value={bestPr?.label || 'none'} note="by PR-AUC" />
        <SummaryTile label="Best ROC-AUC" value={formatScore(bestRoc?.metrics.roc_auc)} note={bestRoc?.label || ''} />
        <SummaryTile label="Best PR-AUC" value={formatScore(bestPr?.metrics.pr_auc)} note={bestPr?.label || ''} />
      </div>

      <section className="panel">
        <PanelTitle title="Metrics table" />
        <div className="metrics-table">
          <div className="metrics-row head">
            <div>Model</div>
            <div>P@10</div>
            <div>P@20</div>
            <div>Recall</div>
            <div>F1</div>
            <div>ROC-AUC</div>
            <div>PR-AUC</div>
          </div>
          {data.rows.map((row) => (
            <MetricRow key={row.scorer_key} row={row} rows={data.rows} />
          ))}
        </div>
      </section>

      <section className="panel">
        <PanelTitle title="Ensemble weights" subtitle="case_ensemble_v1" />
        {data.rows
          .filter((row) => row.ensemble_weights)
          .map((row) => (
            <div key={row.scorer_key} className="weights">
              <Weight label="Transformer" value={row.ensemble_weights?.transformer || 0} />
              <Weight label="LightGBM" value={row.ensemble_weights?.lightgbm || 0} />
              <Weight label="Logistic baseline" value={row.ensemble_weights?.baseline || 0} />
            </div>
          ))}
      </section>
    </section>
  );
}

function SummaryTile({
  label,
  value,
  note,
  tone,
}: {
  label: string;
  value: number | string;
  note: string;
  tone?: RiskLevel;
}) {
  return (
    <div className={`tile ${tone || ''}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{note}</small>
    </div>
  );
}

function SelectFilter({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
}) {
  return (
    <label className="select-chip">
      <span>{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </label>
  );
}

function PanelTitle({ title, subtitle }: { title: string; subtitle?: string }) {
  return (
    <div className="panel-title">
      <h2>{title}</h2>
      {subtitle && <span>{subtitle}</span>}
    </div>
  );
}

function ScoreBar({ value, level, compact = false }: { value: number | null | undefined; level: RiskLevel; compact?: boolean }) {
  const width = Math.max(0, Math.min(1, value || 0)) * 100;
  return (
    <div className={`score-cell ${compact ? 'compact' : ''}`}>
      <div className={`score-track ${riskClass(level)}`}>
        <span style={{ width: `${width}%` }} />
      </div>
      {!compact && <span className="mono">{formatScore(value)}</span>}
    </div>
  );
}

function RiskPill({ level }: { level: RiskLevel }) {
  return <span className={`risk-pill ${riskClass(level)}`}>{level}</span>;
}

function ComponentMini({ scores }: { scores: CaseListItem['scores'] }) {
  return (
    <div className="component-mini">
      <span style={{ height: barHeight(scores.temporal_score) }} />
      <span style={{ height: barHeight(scores.coordination_score) }} />
      <span style={{ height: barHeight(scores.content_score) }} />
      <small>
        {formatScore(scores.temporal_score)} · {formatScore(scores.coordination_score)} · {formatScore(scores.content_score)}
      </small>
    </div>
  );
}

function Sparkline({ data }: { data: number[] }) {
  const max = Math.max(1, ...data);
  const points = data
    .map((value, index) => {
      const x = (index / Math.max(1, data.length - 1)) * 100;
      const y = 24 - (value / max) * 22;
      return `${x},${y}`;
    })
    .join(' ');
  return (
    <svg className="sparkline" viewBox="0 0 100 26" preserveAspectRatio="none">
      <polyline points={points} />
    </svg>
  );
}

function Breakdown({ label, value }: { label: string; value: number | null }) {
  return (
    <div className="breakdown-row">
      <span>{label}</span>
      <div className="breakdown-track">{value !== null && <span style={{ width: `${value * 100}%` }} />}</div>
      <strong className="mono">{formatScore(value)}</strong>
    </div>
  );
}

function EvidenceRow({ item }: { item: CaseDetailsType['evidence'][number] }) {
  return (
    <div className="evidence-row">
      <div>
        <strong>{item.label}</strong>
        <small>
          {item.key} · {item.group}
        </small>
      </div>
      <span className="mono">{item.value === null ? 'value' : formatScore(item.value)}</span>
    </div>
  );
}

function Timeline({ data }: { data: number[] }) {
  const max = Math.max(1, ...data);
  return (
    <div className="timeline">
      {data.map((value, index) => (
        <span key={index} style={{ height: `${Math.max(4, (value / max) * 110)}px` }} />
      ))}
    </div>
  );
}

function ArtifactColumn({ title, items }: { title: string; items: Array<[string, number]> }) {
  return (
    <div>
      <h3>{title}</h3>
      {items.length === 0 && <p className="empty">No values.</p>}
      {items.map(([value, count]) => (
        <div className="artifact-row" key={value}>
          <span>{value}</span>
          <strong>x{count}</strong>
        </div>
      ))}
    </div>
  );
}

function MetricRow({ row, rows }: { row: ModelComparisonRow; rows: ModelComparisonRow[] }) {
  const values = [
    ['precision_at_10', row.metrics.precision_at_10],
    ['precision_at_20', row.metrics.precision_at_20],
    ['recall', row.metrics.recall],
    ['f1', row.metrics.f1],
    ['roc_auc', row.metrics.roc_auc],
    ['pr_auc', row.metrics.pr_auc],
  ] as const;

  return (
    <div className="metrics-row">
      <div className="model-name">
        <strong>{row.label}</strong>
        <small>{row.scorer_key}</small>
      </div>
      {values.map(([key, value]) => (
        <div key={key} className={value === maxMetric(rows, key) ? 'best metric' : 'metric'}>
          {formatMetric(value)}
        </div>
      ))}
    </div>
  );
}

function Weight({ label, value }: { label: string; value: number }) {
  return (
    <div className="weight-row">
      <span>{label}</span>
      <div>
        <span style={{ width: `${value * 100}%` }} />
      </div>
      <strong className="mono">{formatScore(value)}</strong>
    </div>
  );
}

function StateMessage({ title, text }: { title: string; text: string }) {
  return (
    <section className="content">
      <div className="state-message">
        <h1>{title}</h1>
        <p>{text}</p>
      </div>
    </section>
  );
}

function Icon({ name }: { name: string }) {
  const paths: Record<string, React.ReactNode> = {
    queue: <path d="M4 6h16M4 12h16M4 18h16" />,
    chart: <path d="M5 19V5M5 19h15M9 15v-4M13 15V8M17 15v-7" />,
    users: <path d="M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm-5 9c0-4 2-6 5-6s5 2 5 6m3-9a2.5 2.5 0 1 0 0-5m-1 14c0-2 1-4 4-4" />,
    live: <path d="M12 12m-3 0a3 3 0 1 0 6 0 3 3 0 1 0-6 0M5 12a7 7 0 0 1 14 0M2 12a10 10 0 0 1 20 0" />,
    back: <path d="M15 6l-6 6 6 6" />,
  };
  return (
    <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
      {paths[name]}
    </svg>
  );
}

function summarize(items: CaseListItem[]) {
  const total = items.length;
  const high = items.filter((item) => item.risk_level === 'high').length;
  const medium = items.filter((item) => item.risk_level === 'medium').length;
  const low = items.filter((item) => item.risk_level === 'low').length;
  const mean = total ? items.reduce((sum, item) => sum + (item.risk_score || 0), 0) / total : 0;
  return { total, high, medium, low, mean };
}

function scoreToLevel(score: number | null): RiskLevel {
  if ((score || 0) >= 0.7) return 'high';
  if ((score || 0) >= 0.4) return 'medium';
  return 'low';
}

function riskClass(level: RiskLevel) {
  return level === 'medium' ? 'med' : level;
}

function barHeight(value: number | null) {
  return `${4 + (value || 0) * 16}px`;
}

function formatScore(value: number | null | undefined) {
  return value === null || value === undefined ? '-' : value.toFixed(2);
}

function formatMetric(value: number | undefined) {
  return value === undefined ? '-' : value.toFixed(3);
}

function formatPercent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function formatDate(value: string | null) {
  return value ? value.replace('T', ' ').replace('Z', '') : '-';
}

function formatOffset(seconds: number) {
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
}

function scoreLabel(key: string) {
  return SCORERS.find((item) => item.key === key)?.label || key;
}

function bestBy(rows: ModelComparisonRow[], key: keyof ModelComparisonRow['metrics']) {
  return rows.reduce<ModelComparisonRow | null>((best, row) => {
    if (!best || (row.metrics[key] || 0) > (best.metrics[key] || 0)) return row;
    return best;
  }, null);
}

function maxMetric(rows: ModelComparisonRow[], key: keyof ModelComparisonRow['metrics']) {
  return Math.max(...rows.map((row) => row.metrics[key] || 0));
}

export default App;
