import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { SCORERS, getCases, getCasesSummary } from '../../services/api';
import { DEFAULT_SCORER } from '../../constants';
import { ComponentMini, RiskPill, ScoreBar, SelectFilter, Sparkline, StateMessage, SummaryTile } from '../../components/ui';
import { formatScore, summarize } from '../../utils/format';
import styles from './CasesQueue.module.css';

const EMPTY_FILTERS = {
  scorer: DEFAULT_SCORER,
  source: 'pheme_large',
  split: 'eventcv_large',
  risk: 'all',
  event: 'all',
  label: 'all',
  status: 'all',
};

const SOURCE_OPTIONS = ['all', 'pheme_large', 'pheme'];
const SPLIT_OPTIONS = ['all', 'eventcv_large', 'eventcv', 'evalmix', 'smoke'];

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

export function CasesQueue() {
  const navigate = useNavigate();
  const [data, setData] = useState(null);
  const [summaryData, setSummaryData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filters, setFilters] = useState(EMPTY_FILTERS);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const baseQuery = useMemo(
    () => ({
      scorer_key: filters.scorer,
      source_name: filters.source,
      dataset_name: 'pheme',
      dataset_split: filters.split,
      event_name: filters.event,
      risk_level: filters.risk,
      label: filters.label,
      status: filters.status,
      limit: pageSize,
    }),
    [filters, pageSize]
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
  const totalPages = Math.max(1, summaryData?.pages || 1);
  const totalCases = summaryData?.total_cases ?? cases.length;
  const pageStart = totalCases === 0 ? 0 : (currentPage - 1) * pageSize + 1;
  const pageEnd = Math.min(totalCases, currentPage * pageSize);

  if (initialLoading) return <StateMessage title="Loading cases" text="Reading case-level scores from the backend." />;
  if (error && !data) return <StateMessage title="Backend unavailable" text={error} />;

  return (
    <section className={`${styles.root} content`}>
      <div className="page-head">
        <h1>Cases queue</h1>
        <p>
          Ranked suspicious cases with configurable source, split, scorer and analyst filters.
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
        <SelectFilter label="scorer" value={filters.scorer} onChange={(value) => { setFilters((current) => ({ ...current, scorer: value })); setPage(1); }} options={SCORERS.map((scorer) => scorer.key)} />
        <SelectFilter label="source" value={filters.source} onChange={(value) => { setFilters((current) => ({ ...current, source: value })); setPage(1); }} options={SOURCE_OPTIONS} />
        <SelectFilter label="split" value={filters.split} onChange={(value) => { setFilters((current) => ({ ...current, split: value })); setPage(1); }} options={SPLIT_OPTIONS} />
        <SelectFilter label="event" value={filters.event} onChange={(value) => { setFilters((current) => ({ ...current, event: value })); setPage(1); }} options={['all', ...events]} />
        <SelectFilter label="risk" value={filters.risk} onChange={(value) => { setFilters((current) => ({ ...current, risk: value })); setPage(1); }} options={['all', 'high', 'medium', 'low']} />
        <SelectFilter label="label" value={filters.label} onChange={(value) => { setFilters((current) => ({ ...current, label: value })); setPage(1); }} options={['all', 'rumour', 'non-rumour']} />
        <SelectFilter label="status" value={filters.status} onChange={(value) => { setFilters((current) => ({ ...current, status: value })); setPage(1); }} options={['all', 'open', 'suspicious', 'not_suspicious', 'unclear']} />
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

      <PaginationBar
        page={currentPage}
        pages={totalPages}
        loading={loading}
        range={`${pageStart}-${pageEnd}`}
        total={totalCases}
        onFirst={() => setPage(1)}
        onPrev={() => setPage((current) => Math.max(1, current - 1))}
        onNext={() => setPage((current) => Math.min(totalPages, current + 1))}
        onLast={() => setPage(totalPages)}
      />

      <div className="cases-table">
        <div className="case-row table-head">
          <div>id</div>
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
            <div className="case-id-cell">
    <div className="case-id">#{item.id}</div>
    <span className={`status-badge status-${item.status}`}>
      {/* Можно сократить текст для красоты, если нужно */}
      {item.status === 'not_suspicious' ? 'Safe' : 
       item.status === 'suspicious' ? 'Susp' : 
       item.status}
    </span>
  </div>
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

      <PaginationBar
        page={currentPage}
        pages={totalPages}
        loading={loading}
        range={`${pageStart}-${pageEnd}`}
        total={totalCases}
        onFirst={() => setPage(1)}
        onPrev={() => setPage((current) => Math.max(1, current - 1))}
        onNext={() => setPage((current) => Math.min(totalPages, current + 1))}
        onLast={() => setPage(totalPages)}
      />
    </section>
  );
}

function PaginationBar({ page, pages, loading, range, total, onFirst, onPrev, onNext, onLast }) {
  return (
    <div className="pagination pagination-bar unified-pagination">
      <button className="button" disabled={page <= 1 || loading} onClick={onFirst}>First</button>
      <button className="button" disabled={page <= 1 || loading} onClick={onPrev}>Previous</button>
      <span className="mono">Page {page} / {pages} · {range} of {total}{loading ? ' · loading...' : ''}</span>
      <button className="button" disabled={page >= pages || loading} onClick={onNext}>Next</button>
      <button className="button" disabled={page >= pages || loading} onClick={onLast}>Last</button>
    </div>
  );
}
