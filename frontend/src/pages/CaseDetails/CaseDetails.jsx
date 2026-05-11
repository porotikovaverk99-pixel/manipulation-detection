import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getCaseDetails, getCaseScores, recordDecision } from '../../services/api';
import { DEFAULT_SCORER } from '../../constants';
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

export function CaseDetails() {
  const params = useParams();
  const navigate = useNavigate();
  const id = Number(params.id);
  const [scorerKey, setScorerKey] = useState(DEFAULT_SCORER);
  const [details, setDetails] = useState(null);
  const [scores, setScores] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [decision, setDecision] = useState(null);
  const [decisionError, setDecisionError] = useState(null);
  const [decisionLoading, setDecisionLoading] = useState(false);

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
  const rootPost = details.root_post;
  const totalEngagement = details.posts.reduce(
    (sum, post) => sum + (post.likes_count || 0) + (post.reposts_count || 0) + (post.reply_count || 0),
    0
  );
  const verifiedAccounts = details.accounts.filter((account) => account.is_verified).length;
  const topAccounts = details.accounts.slice(0, 8);
  const featureSnapshot = details.features;

    const submitDecision = (value) => {
    setDecisionLoading(true);
    setDecisionError(null);
    recordDecision(details.id, value)
      .then((record) => {
        // Бэкенд может вернуть { decision: 'suspicious' } или просто строку
        const newStatus = record?.decision || value;
        
        setDecision(newStatus);
        
        setDetails((prev) => prev ? { ...prev, status: newStatus } : null);
      })
      .catch((err) => setDecisionError(err.message || 'Failed to record decision'))
      .finally(() => setDecisionLoading(false));
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
            {details.label && <span className="soft-tag">{details.label}</span>}
            <span className={`status-badge status-${details.status}`}>
          {details.status === 'not_suspicious' ? 'Safe' : 
           details.status === 'suspicious' ? 'Susp' : 
           details.status}
        </span>
            
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
              <PanelTitle title="Score breakdown" subtitle={activeScore?.model_version || details.model_version || 'active model'} />
              <Breakdown label="Temporal" value={activeScore?.temporal_score ?? details.scores.temporal_score} />
              <Breakdown label="Coordination" value={activeScore?.coordination_score ?? details.scores.coordination_score} />
              <Breakdown label="Content" value={activeScore?.content_score ?? details.scores.content_score} />
              <Breakdown label="Bot contribution" value={details.scores.bot_score} />
              <div className="score-meta-grid">
                <div>
                  <span>Confidence</span>
                  <strong>{formatScore(activeScore?.confidence_score)}</strong>
                </div>
                <div>
                  <span>Total engagement</span>
                  <strong>{totalEngagement}</strong>
                </div>
                <div>
                  <span>Verified accounts</span>
                  <strong>{verifiedAccounts}</strong>
                </div>
              </div>
            </div>
            <div className={`risk-display ${riskClass(activeScore?.risk_level || details.risk_level)}`}>
              <span>Active risk score</span>
              <strong>{formatScore(activeScore?.risk_score ?? details.risk_score)}</strong>
              <RiskPill level={activeScore?.risk_level || details.risk_level} />
              <small>{details.post_count} posts · {details.duration_min ? `${details.duration_min} min` : 'no time range'}</small>
            </div>
          </section>

          {rootPost && (
            <section className="panel root-post-panel">
              <PanelTitle title="Root post" subtitle="Original claim and engagement" />
              <article className="root-post-card">
                <div className="post-head root-post-head">
                  <span>@{rootPost.author_handle}</span>
                  {rootPost.is_verified && <span>verified</span>}
                  <span>{formatDate(rootPost.published_at)}</span>
                </div>
                <p>{rootPost.text}</p>
                <div className="engagement-grid fancy-engagement-grid">
                  <div className="engagement-card likes-card">
                    <span className="engagement-icon">♥</span>
                    <span className="engagement-label">Likes</span>
                    <strong>{rootPost.likes_count}</strong>
                  </div>
                  <div className="engagement-card reposts-card">
                    <span className="engagement-icon">↻</span>
                    <span className="engagement-label">Reposts</span>
                    <strong>{rootPost.reposts_count}</strong>
                  </div>
                  <div className="engagement-card replies-card">
                    <span className="engagement-icon">↩</span>
                    <span className="engagement-label">Replies</span>
                    <strong>{rootPost.reply_count}</strong>
                  </div>
                  <div className="engagement-card followers-card">
                    <span className="engagement-icon">◉</span>
                    <span className="engagement-label">Followers</span>
                    <strong>{rootPost.followers_count}</strong>
                  </div>
                </div>
                {(rootPost.tags.length > 0 || rootPost.links.length > 0) && (
                  <div className="root-post-artifacts">
                    {rootPost.tags.map((tag) => <span key={`tag-${tag}`}>#{tag.replace(/^#/, '')}</span>)}
                    {rootPost.links.map((link) => <a key={link} href={link} target="_blank" rel="noreferrer">{link}</a>)}
                  </div>
                )}
              </article>
            </section>
          )}

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

          {featureSnapshot && (
            <section className="panel">
              <PanelTitle title="Feature snapshot" subtitle={`${featureSnapshot.feature_version} · computed ${formatDate(featureSnapshot.computed_at)}`} />
              <div className="feature-kpi-grid">
                <div><span>Events</span><strong>{featureSnapshot.event_count}</strong></div>
                <div><span>Accounts</span><strong>{featureSnapshot.unique_account_count}</strong></div>
                <div><span>URLs</span><strong>{featureSnapshot.unique_url_count}</strong></div>
                <div><span>Hashtags</span><strong>{featureSnapshot.unique_hashtag_count}</strong></div>
              </div>
              <div className="feature-columns">
                <FeatureList title="Temporal" values={featureSnapshot.temporal_features} />
                <FeatureList title="Coordination" values={featureSnapshot.coordination_features} />
                <FeatureList title="Content" values={featureSnapshot.content_features} />
              </div>
            </section>
          )}

          <section className="panel timeline-panel">
            <PanelTitle title="Timeline" subtitle="Post density over the case duration" />
            <Timeline data={details.timeline} />
          </section>

          <section className="panel">
            <PanelTitle title="Thread" subtitle={`${details.posts.length} loaded posts`} />
            <div className="thread">
              {details.posts.map((post) => (
                <article key={post.id} className="post enriched-post">
                  <div className="post-time">+{formatOffset(post.t_offset_sec)}</div>
                  <div>
                    <div className="post-head">
                      <span>@{post.author_handle}</span>
                      <span>{post.kind}</span>
                      {post.is_verified && <span>verified</span>}
                    </div>
                    <p>{post.text}</p>
                    <div className="post-metrics compact-engagement-row">
                      <span className="mini-engagement likes-mini">♥ {post.likes_count}</span>
                      <span className="mini-engagement reposts-mini">↻ {post.reposts_count}</span>
                      <span className="mini-engagement replies-mini">↩ {post.reply_count}</span>
                      <span className="mini-engagement followers-mini">◉ {post.followers_count}</span>
                    </div>
                    {(post.tags.length > 0 || post.links.length > 0) && (
                      <div className="post-artifacts">
                        {post.tags.map((tag) => <span key={`tag-${post.id}-${tag}`}>#{tag.replace(/^#/, '')}</span>)}
                        {post.links.map((link) => <a key={`${post.id}-${link}`} href={link} target="_blank" rel="noreferrer">{link}</a>)}
                      </div>
                    )}
                  </div>
                </article>
              ))}
            </div>
          </section>

          <section className="panel">
            <PanelTitle title="Accounts involved" subtitle={`Top ${topAccounts.length} of ${details.accounts.length}`} />
            <div className="accounts-table">
              {topAccounts.map((account) => (
                <div key={account.id} className="account-row enriched-account-row">
                  <div>
                    <strong>@{account.handle}</strong>
                    <small>{account.display_name || 'no display name'} · joined {account.joined_month}</small>
                  </div>
                  <span className="mono">{account.posts} posts</span>
                  <span className="mono">{formatPercent(account.share)}</span>
                  <span className="mono">{account.followers_count} followers</span>
                  <span className={account.has_root_post ? 'soft-tag' : 'soft-tag muted-tag'}>{account.has_root_post ? 'root' : 'reply'}</span>
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
              <button disabled={decisionLoading} onClick={() => submitDecision('open')}>Open</button>
              <button disabled={decisionLoading} onClick={() => submitDecision('suspicious')}>Suspicious</button>
              <button disabled={decisionLoading} onClick={() => submitDecision('not_suspicious')}>Not suspicious</button>
              <button disabled={decisionLoading} onClick={() => submitDecision('unclear')}>Unclear</button>
            </div>
            {decisionLoading && <p className="decision-note">Recording decision…</p>}
            {decision && !decisionLoading && <p className="decision-note success">Decision recorded as {decision}.</p>}
            {decisionError && <p className="decision-note error">{decisionError}</p>}
          </section>

          <section className="panel">
            <PanelTitle title="All scorers" subtitle={`${scores.length} model outputs`} />
            <div className="scorer-list">
              {scores.map((score) => (
                <button key={score.scorer_key} className={score.scorer_key === scorerKey ? 'active' : ''} onClick={() => setScorerKey(score.scorer_key)}>
                  <span>
                    {scoreLabel(score.scorer_key)}
                    <small>{score.scorer_key}</small>
                    <small>{score.model_version}</small>
                  </span>
                  <strong>{formatScore(score.risk_score)}</strong>
                </button>
              ))}
            </div>
          </section>

          <section className="panel">
            <PanelTitle title="Model evidence" subtitle={activeScore?.scorer_key || scorerKey} />
            {activeScore ? (
              <div className="model-evidence">
                <div className="metadata compact-metadata">
                  <div><dt>confidence</dt><dd>{formatScore(activeScore.confidence_score)}</dd></div>
                  <div><dt>source</dt><dd>{activeScore.source_endpoint || '-'}</dd></div>
                  <div><dt>computed</dt><dd>{formatDate(activeScore.computed_at)}</dd></div>
                </div>
                <div className="evidence-list">
                  {(activeScore.evidence || []).map((item) => <div className="evidence-row" key={item}><strong>{item}</strong></div>)}
                </div>
              </div>
            ) : (
              <p className="empty">No model evidence for selected scorer.</p>
            )}
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

function FeatureList({ title, values }) {
  const entries = Object.entries(values || {})
    .filter(([, value]) => typeof value === 'number' && Number.isFinite(value))
    .sort(([, a], [, b]) => Math.abs(Number(b)) - Math.abs(Number(a)))
    .slice(0, 8);

  return (
    <div className="feature-list">
      <h3>{title}</h3>
      {entries.map(([key, value]) => (
        <div className="feature-row" key={key}>
          <span>{key.replace(/_/g, ' ')}</span>
          <strong className="mono">{formatScore(Number(value))}</strong>
        </div>
      ))}
    </div>
  );
}
