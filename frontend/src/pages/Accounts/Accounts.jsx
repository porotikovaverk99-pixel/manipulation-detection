import { useEffect, useMemo, useState } from 'react';
import { getAccounts } from '../../services/api';
import { StateMessage } from '../../components/ui';
import styles from './Accounts.module.css';

const SORT_OPTIONS = [
  ['engagement', 'Engagement'],
  ['posts', 'Posts'],
  ['followers', 'Followers'],
  ['risk', 'Risk'],
  ['manipulation', 'Manipulation'],
];

const PAGE_SIZES = [10, 20, 50, 100];

export function Accounts() {
  const [accounts, setAccounts] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(50);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState('engagement');
  const [datasetSplit, setDatasetSplit] = useState('all');
  const [verified, setVerified] = useState(false);
  const [bots, setBots] = useState(false);

  useEffect(() => {
    setPage(1);
  }, [search, sort, datasetSplit, verified, bots, limit]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    getAccounts({
      dataset_name: 'pheme',
      dataset_split: datasetSplit,
      search,
      sort,
      verified,
      bots,
      page,
      limit,
    })
      .then((payload) => {
        if (!cancelled) {
          setAccounts(payload.items || []);
          setTotal(payload.total || 0);
        }
      })
      .catch((err) => {
        if (!cancelled) setError(err.message || 'Failed to load accounts.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [search, sort, datasetSplit, verified, bots, page, limit]);

  const pages = Math.max(1, Math.ceil(total / limit));
  const pageStart = total === 0 ? 0 : (page - 1) * limit + 1;
  const pageEnd = Math.min(total, page * limit);
  const summary = useMemo(() => summarizeAccounts(accounts), [accounts]);

  if (loading && accounts.length === 0) return <StateMessage title="Loading accounts" text="Aggregating account-level signals." />;
  if (error) return <StateMessage title="Accounts unavailable" text={error} />;

  return (
    <section className={`${styles.root} content accounts-page`}>
      <div className="page-head accounts-headline">
        <div>
          <h1>Accounts</h1>
          <p>Account-level activity, engagement, risk and manipulation coverage across posts and cases.</p>
        </div>
      </div>

      <div className="accounts-toolbar">
        <label className="search-box">
          <span>Search</span>
          <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="username, display name, external id" />
        </label>
        <label className="select-chip">
          <span>split</span>
          <select value={datasetSplit} onChange={(event) => setDatasetSplit(event.target.value)}>
            {['all', 'eventcv_large', 'eventcv'].map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>sort</span>
          <select value={sort} onChange={(event) => setSort(event.target.value)}>
            {SORT_OPTIONS.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <label className="select-chip">
          <span>page size</span>
          <select value={limit} onChange={(event) => setLimit(Number(event.target.value))}>
            {PAGE_SIZES.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="toggle-chip"><input type="checkbox" checked={verified} onChange={(event) => setVerified(event.target.checked)} /> Verified</label>
        <label className="toggle-chip"><input type="checkbox" checked={bots} onChange={(event) => setBots(event.target.checked)} /> Bots</label>
      </div>

            <div className="pagination unified-pagination">
        <button className="button" disabled={page <= 1 || loading} onClick={() => setPage(1)}>First</button>
        <button className="button" disabled={page <= 1 || loading} onClick={() => setPage((current) => Math.max(1, current - 1))}>Previous</button>
        <span className="mono">
          Page {page} / {pages} · {pageStart}-{pageEnd} of {total}
          {loading ? ' · loading...' : ''}
        </span>
        <button className="button" disabled={page >= pages || loading} onClick={() => setPage((current) => Math.min(pages, current + 1))}>Next</button>
        <button className="button" disabled={page >= pages || loading} onClick={() => setPage(pages)}>Last</button>
      </div>

      <div className="accounts-table rich-table">
        <div className="accounts-row accounts-head">
          <span>Account</span>
          <span>Posts</span>
          <span>Cases</span>
          <span>Engagement</span>
          <span>Max manip.</span>
          <span>Avg risk</span>
          <span>Followers</span>
        </div>
        {accounts.map((account) => (
          <div className="accounts-row" key={account.id}>
            <div className="account-main-cell">
              <strong>@{account.username || account.external_id}</strong>
              <small>
                {account.display_name || 'no display name'}
                {account.is_verified ? ' · verified' : ''}
                {account.is_bot ? ' · bot' : ''}
                {account.first_seen_at ? ` · first ${formatDate(account.first_seen_at)}` : ''}
              </small>
            </div>
            <span className="mono metric-stack"><strong>{formatNumber(account.dataset_post_count)}</strong><small>{account.root_post_count} root</small></span>
            <span className="mono metric-stack"><strong>{formatNumber(account.case_count)}</strong><small>{account.high_risk_case_count} high-risk</small></span>
            <span className="mono">{formatNumber(account.total_engagement)}</span>
            <MetricPill value={account.max_manipulation} />
            <MetricPill value={account.avg_risk_score} />
            <span className="mono metric-stack"><strong>{formatNumber(account.followers_count)}</strong><small>{formatNumber(account.following_count)} following</small></span>
          </div>
        ))}
        {accounts.length === 0 && <div className="accounts-empty">No accounts match current filters.</div>}
      </div>

            <div className="pagination unified-pagination">
        <button className="button" disabled={page <= 1 || loading} onClick={() => setPage(1)}>First</button>
        <button className="button" disabled={page <= 1 || loading} onClick={() => setPage((current) => Math.max(1, current - 1))}>Previous</button>
        <span className="mono">
          Page {page} / {pages} · {pageStart}-{pageEnd} of {total}
          {loading ? ' · loading...' : ''}
        </span>
        <button className="button" disabled={page >= pages || loading} onClick={() => setPage((current) => Math.min(pages, current + 1))}>Next</button>
        <button className="button" disabled={page >= pages || loading} onClick={() => setPage(pages)}>Last</button>
      </div>
    </section>
  );
}

function AccountKpi({ label, value, hint, tone = 'neutral' }) {
  return (
    <div className={`account-kpi ${tone}`}>
      <span>{label}</span>
      <strong>{typeof value === 'number' ? formatNumber(value) : value}</strong>
      <small>{hint}</small>
    </div>
  );
}

function MetricPill({ value }) {
  const level = scoreTone(value);
  return <span className={`metric-pill ${level}`}>{Number(value || 0).toFixed(2)}</span>;
}

function summarizeAccounts(items) {
  return items.reduce(
    (acc, item) => ({
      posts: acc.posts + (item.dataset_post_count || 0),
      cases: acc.cases + (item.case_count || 0),
      engagement: acc.engagement + (item.total_engagement || 0),
      verified: acc.verified + (item.is_verified ? 1 : 0),
      maxManipulation: Math.max(acc.maxManipulation, item.max_manipulation || 0),
    }),
    { posts: 0, cases: 0, engagement: 0, verified: 0, maxManipulation: 0 }
  );
}

function scoreTone(value) {
  if ((value || 0) >= 0.7) return 'high';
  if ((value || 0) >= 0.4) return 'med';
  return 'low';
}

function formatNumber(value) {
  return new Intl.NumberFormat('en-US').format(Math.round(Number(value || 0)));
}

function formatDate(value) {
  return value ? value.slice(0, 10) : '-';
}
