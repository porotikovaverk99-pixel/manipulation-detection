// Типы данных для фронтенда

export interface Account {
  id: number;
  externalId: string;
  username: string;
  displayName: string;
  followersCount: number;
  avatarUrl: string;
}

export interface Post {
  id: number;
  externalId: string;
  content: string;
  language: string;
  publishedAt: string;
  likesCount: number;
  repostsCount: number;
  author: Account;
}

export interface AnalysisResult {
  id: number;
  postId: number;
  manipulationScore: number;
  confidenceScore: number;
  coordinationContribution: number;
  temporalContribution: number;
  narrativeContribution: number;
  escalationPriority: number;
  confidenceNote: string;
}

export interface EvidenceCard {
  id: number;
  analysisResultId: number;
  radarData: {
    coordination: number;
    temporal: number;
    narrative: number;
  };
  summary: string;
  keyEvidence: string[];
  uncertaintyExplanation: string;
}

export interface TrendingTag {
  id: number;
  tagName: string;
  todayAccounts: number;
  todayUses: number;
}