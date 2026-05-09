import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { getCases, getCasesSummary } from '../../services/api';
import { ComponentMini, RiskPill, ScoreBar, SelectFilter, Sparkline, StateMessage, SummaryTile } from '../../components/ui';
import { formatScore, summarize } from '../../utils/format';
import styles from './CasesQueue.module.css';

const EMPTY_FILTERS = {
  risk: 'all',
  event: 'all',
  label: 'all',
  status: 'all',
};

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

export function CasesQueue({ scorerKey }) {
  const navigate = useNavigate();
  const [data, setData] = useState(null);
  const [summaryData, setSummaryData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filters, setFilters] = useState(EMPTY_FILTERS);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  useEffect(() => {
    setPage(1);
  }, [scorerKey]);

  const baseQuery = useMemo(
    () => ({
      scorer_key: scorerKey,
      source_name: 'pheme_large',
      dataset_name: 'pheme',
      dataset_split: 'eventcv_large',
      event_name: filters.event,
      risk_level: filters.risk,
      label: filters.label,
      status: filters.status,
      limit: pageSize,
    }),
    [scorerKey, filters, pageSize]
  );

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([
      getCases({ ...baseQuery, page }),
      getCasesSummary(baseQuery),
    ])
      .then(([casesResponse, summaryResponse]) => {
        if (!cancelled) {
          setData(casesResponse);
          setSummaryData(summaryResponse);
        }
      })
      .catch((err) => {
        if (!cancelled) setError(err.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [baseQuery, page]);

  const cases = useMemo(() => data?.items || [], [data]);
  const events = useMemo(() => Array.from(new Set(cases.map((item) => item.event_name))).sort(), [cases]);
  const fallbackSummary = summarize(cases);
  const initialLoading = loading && !data && !summaryData;
  const currentPage = data?.page || page;
  const totalPages = summaryData?.pages || 0;
  const canGoNext = totalPages ? currentPage < totalPages : false;

  if (initialLoading) return <StateMessage title="Loading cases" text="Reading case-level scores from the backend." />;
  if (error && !data) return <StateMessage title="Backend unavailable" text={error} />;

  return (
    <section className={`${styles.root} content`}>
      <div className="page-head">
        <h1>Cases queue</h1>
        <p>
          Ranked suspicious cases from <span className="mono">pheme / eventcv_large</span>. The active scorer is{' '}
          <span className="mono accent">{scorerKey}</span>.
        </p>
      </div>

      <div className="tiles">
        <SummaryTile label="Cases" value={summaryData?.total_cases ?? cases.length} note={totalPages ? `${totalPages} pages` : `${cases.length} loaded`} />
        <SummaryTile label="High risk" value={summaryData?.high_risk ?? fallbackSummary.high} note="score >= 0.70" tone="high" />
        <SummaryTile label="Medium risk" value={summaryData?.medium_risk ?? fallbackSummary.medium} note="0.40 to 0.70" tone="medium" />
        <SummaryTile label="Low risk" value={summaryData?.low_risk ?? fallbackSummary.low} note="score < 0.40" tone="low" />
        <SummaryTile label="Mean risk" value={formatScore(summaryData?.mean_risk ?? fallbackSummary.mean)} note="filtered queue" />
      </div>

      <div className="filters">
        <SelectFilter label="event" value={filters.event} onChange={(value) => { setFilters((current) => ({ ...current, event: value })); setPage(1); }} options={['all', ...events]} />
        <SelectFilter label="risk" value={filters.risk} onChange={(value) => { setFilters((current) => ({ ...current, risk: value })); setPage(1); }} options={['all', 'high', 'medium', 'low']} />
        <SelectFilter label="label" value={filters.label} onChange={(value) => { setFilters((current) => ({ ...current, label: value })); setPage(1); }} options={['all', 'rumour', 'non-rumour']} />
        <SelectFilter label="status" value={filters.status} onChange={(value) => { setFilters((current) => ({ ...current, status: value })); setPage(1); }} options={['all', 'open', 'closed', 'finalized', 'new', 'in_review', 'decided']} />
        <SelectFilter
          label="page size"
          value={String(pageSize)}
          onChange={(value) => {
            setPageSize(Number(value));
            setPage(1);
          }}
          options={PAGE_SIZE_OPTIONS.map(String)}
        />
        <button className="button ghost" onClick={() => { setFilters(EMPTY_FILTERS); setPage(1); }}>Clear</button>
      </div>

      <div className="pagination">
        <button className="button" disabled={page <= 1 || loading} onClick={() => setPage((current) => Math.max(1, current - 1))}>
          Previous
        </button>
        <span className="mono">Page {currentPage}{totalPages ? ` / ${totalPages}` : ''}{loading ? ' · loading...' : ''}</span>
        <button className="button" disabled={!canGoNext || loading} onClick={() => setPage((current) => current + 1)}>
          Next
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
        {cases.map((item) => (
          <button key={item.id} className="case-row data-row" onClick={() => navigate(`/cases/${item.id}`)}>
            <div className="case-id">#{item.id}</div>
            <div className="case-title">
              <span>{item.title}</span>
              <small>{item.event_name} · {item.dataset_name}/{item.dataset_split}</small>
            </div>
            <div className="muted">
              <div>{item.post_count} posts</div>
              <div className="mono">{item.duration_min ? `${item.duration_min}m` : 'no range'}</div>
            </div>
            <ScoreBar value={item.risk_score} level={item.risk_level} />
            <RiskPill level={item.risk_level} />
            <ComponentMini scores={item.scores} />
            <div className="evidence-tags">
              {item.evidence.slice(0, 2).map((evidence) => <span key={evidence.key}>{evidence.key}</span>)}
            </div>
            <Sparkline data={item.timeline} />
          </button>
        ))}
      </div>

      <div className="pagination">
        <button className="button" disabled={page <= 1 || loading} onClick={() => setPage((current) => Math.max(1, current - 1))}>
          Previous
        </button>
        <span className="mono">Page {currentPage}{totalPages ? ` / ${totalPages}` : ''}{loading ? ' · loading...' : ''}</span>
        <button className="button" disabled={!canGoNext || loading} onClick={() => setPage((current) => current + 1)}>
          Next
        </button>
      </div>
    </section>
  );
}
