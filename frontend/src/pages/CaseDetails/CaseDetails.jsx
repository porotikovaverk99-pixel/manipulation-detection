import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getCaseDetails, getCaseScores, recordDecision } from '../../services/api';
import {
  ArtifactColumn,
  Breakdown,
  EvidenceRow,
  Icon,
  PanelTitle,
  RiskPill,
  ScoreBar,
  StateMessage,
  Timeline,
} from '../../components/ui';
import { formatDate, formatOffset, formatPercent, formatScore, riskClass, scoreLabel, scoreToLevel } from '../../utils/format';
import styles from './CaseDetails.module.css';

export function CaseDetails({ scorerKey, setScorerKey }) {
  const params = useParams();
  const navigate = useNavigate();
  const id = Number(params.id);
  const [details, setDetails] = useState(null);
  const [scores, setScores] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [decision, setDecision] = useState(null);

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
      .catch((err) => {
        if (!cancelled) setError(err.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id, scorerKey]);

  if (loading) return <StateMessage title="Loading case" text={`Reading case #${id}.`} />;
  if (error || !details) return <StateMessage title="Case unavailable" text={error || 'Case not found.'} />;

  const activeScore = scores.find((score) => score.scorer_key === scorerKey);

  const submitDecision = (value) => {
    setDecision(value);
    recordDecision(details.id, value).catch(() => undefined);
  };

  return (
    <section className={`${styles.root} content`}>
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
            Event <span className="mono">{details.event_name}</span> · source <span className="mono">{details.source_name}</span> · dataset{' '}
            <span className="mono">{details.dataset_name}/{details.dataset_split}</span>
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
              <small>{details.post_count} posts · {details.duration_min ? `${details.duration_min} min` : 'no time range'}</small>
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
                <button key={score.scorer_key} className={score.scorer_key === scorerKey ? 'active' : ''} onClick={() => setScorerKey(score.scorer_key)}>
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
