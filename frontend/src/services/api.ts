import {
  AnalystDecision,
  CaseArtifacts,
  CaseDetails,
  CaseFeaturesSnapshot,
  CaseListItem,
  CaseModelScore,
  CaseScoresResponse,
  CasesListQuery,
  CasesListResponse,
  ComponentScores,
  DecisionResponse,
  EvidenceGroup,
  EvidenceItem,
  ModelComparisonResponse,
  ModelComparisonRow,
  RiskLevel,
  ScorerKey,
  ScorerOption,
} from '../types';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:8080/api';

export const SCORERS: ScorerOption[] = [
  {
    key: 'case_ensemble_v1',
    label: 'Ensemble v1',
    description: 'Weighted blend of transformer and case-feature models.',
  },
  {
    key: 'pheme_transformer_text_oof',
    label: 'Transformer text',
    description: 'DistilRoBERTa over root post and reactions.',
  },
  {
    key: 'case_lightgbm_oof',
    label: 'LightGBM',
    description: 'Gradient boosting over engineered case features.',
  },
  {
    key: 'case_logreg_oof',
    label: 'Logistic baseline',
    description: 'Interpretable linear baseline over case features.',
  },
  {
    key: 'case_feature',
    label: 'Feature heuristic',
    description: 'Current active engineered-feature score.',
  },
  {
    key: 'case_scores',
    label: 'Active score',
    description: 'Legacy active score from case_scores.',
  },
];

interface BackendCaseListItem {
  id: number;
  source_name: string;
  dataset_name: string;
  dataset_split: string;
  external_case_id: string;
  case_type: string;
  label?: string;
  title?: string;
  event_name?: string;
  status: string;
  first_event_at?: string;
  last_event_at?: string;
  post_count: number;
  scorer_key?: string;
  model_version?: string;
  risk_score?: number;
  risk_level?: string;
  temporal_score?: number;
  coordination_score?: number;
  content_score?: number;
}

interface BackendCasesListResponse {
  count: number;
  items: BackendCaseListItem[];
}

interface BackendPostItem {
  id: number;
  external_id: string;
  account_id: number;
  username: string;
  published_at: string;
  content: string;
  is_case_root: boolean;
  reply_to_post_id?: number;
  replies_count?: number;
  tags?: string[];
  links?: string[];
}

interface BackendAccountItem {
  id: number;
  external_id: string;
  username: string;
  case_post_count: number;
  first_post_at?: string;
  is_bot: boolean;
  is_verified: boolean;
}

interface BackendArtifactCount {
  value: string;
  count: number;
}

interface BackendCaseArtifacts {
  tags?: BackendArtifactCount[];
  urls?: BackendArtifactCount[];
  domains?: BackendArtifactCount[];
}

interface BackendCaseDetailsResponse {
  case: BackendCaseListItem;
  root_post?: BackendPostItem;
  posts: BackendPostItem[];
  accounts: BackendAccountItem[];
  artifacts: BackendCaseArtifacts;
  features?: CaseFeaturesSnapshot;
}

interface BackendCaseScoresResponse {
  case_id: number;
  count: number;
  items: CaseModelScore[];
}

interface BackendModelComparison {
  dataset: {
    source_name?: string;
    dataset_name?: string;
    dataset_split?: string;
  };
  case_count: number;
  positive_cases: number;
  negative_cases: number;
  models: Record<
    string,
    {
      scorer_key: string;
      model_version?: string;
      case_count: number;
      metrics: Record<string, number | undefined>;
    }
  >;
  generated_at: string;
}

export async function getCases(query: CasesListQuery = {}): Promise<CasesListResponse> {
  const params = toSearchParams({
    ...query,
    limit: query.limit ?? 100,
  });
  const payload = await requestJSON<BackendCasesListResponse>(`/cases?${params.toString()}`);
  const scorerKey = query.scorer_key || 'case_ensemble_v1';

  return {
    scorer_key: scorerKey,
    total: payload.count,
    items: payload.items.map((item) => mapCaseListItem(item, scorerKey)),
  };
}

export async function getCaseDetails(id: number, scorerKey: ScorerKey): Promise<CaseDetails> {
  const payload = await requestJSON<BackendCaseDetailsResponse>(
    `/cases/${id}?${toSearchParams({ scorer_key: scorerKey }).toString()}`
  );
  return mapCaseDetails(payload, scorerKey);
}

export async function getCaseScores(id: number): Promise<CaseScoresResponse> {
  const payload = await requestJSON<BackendCaseScoresResponse>(`/cases/${id}/scores`);
  return {
    case_id: payload.case_id,
    count: payload.count,
    items: payload.items,
  };
}

export async function getModelComparison(): Promise<ModelComparisonResponse> {
  const params = toSearchParams({
    source_name: 'pheme_large',
    dataset_name: 'pheme',
    dataset_split: 'eventcv_large',
    top_k: 20,
  });
  const payload = await requestJSON<BackendModelComparison>(`/model-comparison?${params.toString()}`);
  const rows = Object.values(payload.models || {})
    .map(mapModelComparisonRow)
    .sort((a, b) => (b.metrics.pr_auc || 0) - (a.metrics.pr_auc || 0));

  return {
    source_name: payload.dataset?.source_name || '',
    dataset_name: payload.dataset?.dataset_name || '',
    dataset_split: payload.dataset?.dataset_split || '',
    case_count: payload.case_count,
    positive_cases: payload.positive_cases,
    negative_cases: payload.negative_cases,
    rows,
    generated_at: payload.generated_at,
  };
}

export async function recordDecision(id: number, decision: AnalystDecision): Promise<DecisionResponse> {
  const response = await fetch(`${API_BASE_URL}/cases/${id}/decision`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ decision }),
  });

  if (response.status === 404 || response.status === 405) {
    return {
      case_id: id,
      decision,
      recorded_at: new Date().toISOString(),
      audit_id: 'local-preview',
    };
  }

  if (!response.ok) {
    throw new Error(`Decision request failed with HTTP ${response.status}`);
  }

  return response.json();
}

async function requestJSON<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`);
  if (!response.ok) {
    throw new Error(`API request failed with HTTP ${response.status}: ${path}`);
  }
  return response.json() as Promise<T>;
}

function toSearchParams(values: Record<string, unknown>): URLSearchParams {
  const params = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '' || value === 'all') {
      return;
    }
    params.set(key, String(value));
  });
  return params;
}

function mapCaseListItem(item: BackendCaseListItem, scorerKey: string): CaseListItem {
  const first = item.first_event_at || null;
  const last = item.last_event_at || null;
  const duration = computeDurationMin(first, last);
  const score = item.risk_score ?? null;
  const level = toRiskLevel(item.risk_level, score);
  const scores: ComponentScores = {
    temporal_score: item.temporal_score ?? null,
    coordination_score: item.coordination_score ?? null,
    content_score: item.content_score ?? null,
    bot_score: null,
  };

  return {
    id: item.id,
    external_id: item.external_case_id,
    title: item.title || item.external_case_id || `Case ${item.id}`,
    event_name: item.event_name || 'unknown',
    source_name: item.source_name,
    dataset_name: item.dataset_name,
    dataset_split: item.dataset_split,
    label: normalizeLabel(item.label),
    status: normalizeStatus(item.status),
    post_count: item.post_count,
    duration_min: duration,
    first_event_at: first,
    last_event_at: last,
    active_scorer: item.scorer_key || scorerKey,
    model_version: item.model_version || '',
    risk_score: score,
    risk_level: level,
    scores,
    evidence: evidenceFromComponents(scores),
    timeline: syntheticTimeline(item.id, item.post_count, score ?? 0.4),
  };
}

function mapCaseDetails(payload: BackendCaseDetailsResponse, scorerKey: string): CaseDetails {
  const base = mapCaseListItem(payload.case, scorerKey);
  const start = base.first_event_at ? Date.parse(base.first_event_at) : null;
  const posts = (payload.posts || []).map((post) => mapPost(post, start));
  const postCount = Math.max(1, base.post_count);
  const features = payload.features || null;
  const evidence = features ? evidenceFromFeatures(features) : base.evidence;

  return {
    ...base,
    evidence,
    timeline: timelineFromPosts(posts),
    root_post: payload.root_post ? mapPost(payload.root_post, start) : posts.find((post) => post.kind === 'root') || null,
    posts,
    accounts: (payload.accounts || []).map((account) => ({
      id: account.id,
      external_id: account.external_id,
      handle: account.username,
      joined_month: account.first_post_at ? account.first_post_at.slice(0, 7) : 'unknown',
      posts: account.case_post_count,
      share: account.case_post_count / postCount,
      bot_score: account.is_bot ? 0.7 : null,
      is_verified: account.is_verified,
    })),
    artifacts: mapArtifacts(payload.artifacts || {}),
    features,
  };
}

function mapPost(post: BackendPostItem, startMs: number | null): import('../types').PostItem {
  const published = Date.parse(post.published_at);
  const offset = startMs && Number.isFinite(published) ? Math.max(0, Math.round((published - startMs) / 1000)) : 0;
  return {
    id: String(post.id),
    external_id: post.external_id,
    author_handle: post.username,
    t_offset_sec: offset,
    kind: post.is_case_root ? 'root' : post.reply_to_post_id ? 'reply' : 'repost',
    text: post.content,
    reply_count: post.replies_count || 0,
    published_at: post.published_at,
    tags: post.tags || [],
    links: post.links || [],
  };
}

function mapArtifacts(artifacts: BackendCaseArtifacts): CaseArtifacts {
  return {
    urls: (artifacts.urls || []).map((item) => ({
      url: item.value,
      domain: extractDomain(item.value),
      count: item.count,
    })),
    hashtags: (artifacts.tags || []).map((item) => ({
      tag: item.value.startsWith('#') ? item.value : `#${item.value}`,
      count: item.count,
    })),
    phrases: [],
  };
}

function mapModelComparisonRow(item: BackendModelComparison['models'][string]): ModelComparisonRow {
  const key = item.scorer_key;
  return {
    scorer_key: key,
    label: scorerLabel(key),
    description: scorerDescription(key),
    model_version: item.model_version || '',
    case_count: item.case_count,
    metrics: {
      best_threshold: item.metrics.best_threshold,
      precision_at_10: item.metrics.precision_at_10,
      precision_at_20: item.metrics.precision_at_20,
      precision: item.metrics.precision,
      recall: item.metrics.recall,
      f1: item.metrics.f1,
      roc_auc: item.metrics.roc_auc,
      pr_auc: item.metrics.pr_auc,
    },
    ensemble_weights:
      key === 'case_ensemble_v1'
        ? {
            transformer: 0.65,
            lightgbm: 0.35,
            baseline: 0,
          }
        : undefined,
  };
}

function evidenceFromComponents(scores: ComponentScores): EvidenceItem[] {
  const items: Array<[EvidenceGroup, keyof ComponentScores, string]> = [
    ['temporal', 'temporal_score', 'Temporal score'],
    ['coordination', 'coordination_score', 'Coordination score'],
    ['content', 'content_score', 'Content score'],
  ];
  return items
    .map(([group, key, label]) => ({
      key,
      label,
      group,
      value: scores[key],
      baseline: 0.5,
      dir: 'up' as const,
    }))
    .filter((item) => item.value !== null)
    .sort((a, b) => (b.value || 0) - (a.value || 0));
}

function evidenceFromFeatures(features: CaseFeaturesSnapshot): EvidenceItem[] {
  const groups: Array<[EvidenceGroup, Record<string, unknown>]> = [
    ['temporal', features.temporal_features],
    ['coordination', features.coordination_features],
    ['content', features.content_features],
  ];

  const items = groups.flatMap(([group, values]) =>
    Object.entries(values || {})
      .filter(([, value]) => typeof value === 'number' && Number.isFinite(value))
      .map(([key, value]) => ({
        key,
        label: humanizeKey(key),
        group,
        value: Number(value),
        baseline: null,
        dir: 'up' as const,
      }))
  );

  return items.sort((a, b) => Math.abs(b.value || 0) - Math.abs(a.value || 0)).slice(0, 8);
}

function timelineFromPosts(posts: import('../types').PostItem[]): number[] {
  if (posts.length === 0) {
    return syntheticTimeline(0, 0, 0.4);
  }
  const maxOffset = Math.max(...posts.map((post) => post.t_offset_sec), 1);
  const buckets = Array.from({ length: 40 }, () => 0);
  posts.forEach((post) => {
    const index = Math.min(39, Math.floor((post.t_offset_sec / maxOffset) * 39));
    buckets[index] += 1;
  });
  return buckets;
}

function syntheticTimeline(seed: number, count: number, risk: number): number[] {
  const buckets = 40;
  const peak = Math.max(3, Math.min(24, Math.floor((seed % 17) + risk * 12)));
  return Array.from({ length: buckets }, (_, index) => {
    const distance = Math.abs(index - peak);
    const shape = Math.max(0, 1 - distance / 10);
    const base = Math.max(1, Math.round((count / 18) * shape));
    return index % 11 === seed % 11 ? base + 1 : base;
  });
}

function computeDurationMin(first: string | null, last: string | null): number | null {
  if (!first || !last) {
    return null;
  }
  const delta = Date.parse(last) - Date.parse(first);
  if (!Number.isFinite(delta) || delta < 0) {
    return null;
  }
  return Math.max(1, Math.round(delta / 60000));
}

function normalizeStatus(status: string): CaseListItem['status'] {
  if (status === 'open' || status === 'closed' || status === 'finalized') {
    return status;
  }
  if (status === 'new' || status === 'in_review' || status === 'decided') {
    return status;
  }
  return 'closed';
}

function normalizeLabel(label?: string): string | null {
  if (!label) {
    return null;
  }
  return label.replace('_', '-');
}

function toRiskLevel(value?: string, score?: number | null): RiskLevel {
  if (value === 'high' || value === 'medium' || value === 'low') {
    return value;
  }
  if ((score || 0) >= 0.7) {
    return 'high';
  }
  if ((score || 0) >= 0.4) {
    return 'medium';
  }
  return 'low';
}

function extractDomain(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url.split('/')[0] || url;
  }
}

function humanizeKey(key: string): string {
  return key
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

function scorerLabel(key: string): string {
  return SCORERS.find((item) => item.key === key)?.label || key;
}

function scorerDescription(key: string): string {
  return SCORERS.find((item) => item.key === key)?.description || 'Model score from backend.';
}
