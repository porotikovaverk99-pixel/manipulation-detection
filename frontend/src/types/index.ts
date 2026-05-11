export type RiskLevel = 'high' | 'medium' | 'low';

export type CaseStatus = 'open' | 'suspicious' | 'not_suspicious' | 'unclear';

export type ScorerKey =
  | 'case_scores'
  | 'case_feature'
  | 'case_logreg_oof'
  | 'case_lightgbm_oof'
  | 'pheme_transformer_text'
  | 'pheme_transformer_text_oof'
  | 'case_ensemble_v1';

export type EvidenceGroup = 'temporal' | 'coordination' | 'content' | 'bot' | 'model';

export interface ComponentScores {
  temporal_score: number | null;
  coordination_score: number | null;
  content_score: number | null;
  bot_score: number | null;
}

export interface EvidenceItem {
  key: string;
  label: string;
  group: EvidenceGroup;
  value: number | null;
  baseline: number | null;
  dir: 'up' | 'down';
}

export interface CaseListItem {
  id: number;
  external_id: string;
  title: string;
  event_name: string;
  source_name: string;
  dataset_name: string;
  dataset_split: string;
  label: string | null;
  status: CaseStatus;
  post_count: number;
  duration_min: number | null;
  first_event_at: string | null;
  last_event_at: string | null;
  active_scorer: string;
  model_version: string;
  risk_score: number | null;
  risk_level: RiskLevel;
  scores: ComponentScores;
  evidence: EvidenceItem[];
  timeline: number[];
}

export interface CasesListQuery {
  scorer_key?: ScorerKey;
  source_name?: string;
  dataset_name?: string;
  dataset_split?: string;
  event_name?: string;
  status?: CaseStatus | 'all';
  risk_level?: RiskLevel | 'all';
  label?: string;
  limit?: number;
  page?: number;
  days?: number;
}

export interface CasesListResponse {
  scorer_key: string;
  total: number;
  items: CaseListItem[];
  page: number;
  limit: number;
}

export interface CasesSummaryResponse {
  total_cases: number;
  high_risk: number;
  medium_risk: number;
  low_risk: number;
  mean_risk: number;
  limit: number;
  pages: number;
}

export interface PostItem {
  id: string;
  external_id: string;
  account_id: number;
  author_handle: string;
  t_offset_sec: number;
  kind: 'root' | 'reply' | 'repost';
  text: string;
  reply_count: number;
  published_at: string;
  tags: string[];
  links: string[];
  likes_count: number;
  reposts_count: number;
  followers_count: number;
  following_count: number;
  is_verified: boolean;
}

export interface AccountInvolved {
  id: number;
  external_id: string;
  handle: string;
  display_name: string;
  joined_month: string;
  posts: number;
  followers_count: number;
  following_count: number;
  share: number;
  bot_score: number | null;
  is_verified: boolean;
  has_root_post: boolean;
}

export interface UrlArtifact {
  url: string;
  domain: string;
  count: number;
}

export interface HashtagArtifact {
  tag: string;
  count: number;
}

export interface PhraseArtifact {
  phrase: string;
  count: number;
}

export interface CaseArtifacts {
  urls: UrlArtifact[];
  hashtags: HashtagArtifact[];
  phrases: PhraseArtifact[];
}

export interface CaseFeaturesSnapshot {
  feature_version: string;
  event_count: number;
  unique_account_count: number;
  unique_url_count: number;
  unique_hashtag_count: number;
  temporal_features: Record<string, unknown>;
  coordination_features: Record<string, unknown>;
  content_features: Record<string, unknown>;
  feature_payload: Record<string, unknown>;
  computed_at: string;
}

export interface CaseDetails extends CaseListItem {
  root_post: PostItem | null;
  posts: PostItem[];
  accounts: AccountInvolved[];
  artifacts: CaseArtifacts;
  features: CaseFeaturesSnapshot | null;
}

export interface CaseModelScore {
  case_id: number;
  scorer_key: string;
  model_version: string;
  risk_score: number;
  risk_level: RiskLevel;
  confidence_score: number | null;
  temporal_score: number | null;
  coordination_score: number | null;
  content_score: number | null;
  evidence: string[];
  feature_payload: Record<string, unknown>;
  model_info: Record<string, unknown>;
  pipeline_hash: string;
  source_endpoint: string;
  computed_at: string;
}

export interface CaseScoresResponse {
  case_id: number;
  count: number;
  items: CaseModelScore[];
}

export interface ModelMetrics {
  best_threshold?: number;
  precision_at_10?: number;
  precision_at_20?: number;
  precision?: number;
  recall?: number;
  f1?: number;
  roc_auc?: number;
  pr_auc?: number;
}

export interface ModelComparisonRow {
  scorer_key: string;
  label: string;
  description: string;
  model_version: string;
  case_count: number;
  metrics: ModelMetrics;
  ensemble_weights?: {
    transformer: number;
    lightgbm: number;
    baseline: number;
  };
}

export interface ModelComparisonResponse {
  dataset_name: string;
  dataset_split: string;
  source_name: string;
  case_count: number;
  positive_cases: number;
  negative_cases: number;
  rows: ModelComparisonRow[];
  generated_at: string;
}

export type AnalystDecision = 'suspicious' | 'not_suspicious' | 'unclear';

export interface DecisionResponse {
  case_id: number;
  decision: AnalystDecision;
  recorded_at: string;
  audit_id: string;
}

export interface ScorerOption {
  key: ScorerKey;
  label: string;
  description: string;
}
