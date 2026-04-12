import axios from 'axios';
import { Post, AnalysisResult, EvidenceCard, TrendingTag } from '../types';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:8080/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Посты
export const getPosts = async (limit: number = 50): Promise<Post[]> => {
  const response = await api.get(`/posts?limit=${limit}`);
  return response.data;
};

export const getPost = async (id: number): Promise<Post> => {
  const response = await api.get(`/posts/${id}`);
  return response.data;
};

// Результаты анализа
export const getAnalysisResults = async (limit: number = 50): Promise<AnalysisResult[]> => {
  const response = await api.get(`/analysis?limit=${limit}`);
  return response.data;
};

// Evidence Cards
export const getEvidenceCard = async (analysisId: number): Promise<EvidenceCard> => {
  const response = await api.get(`/evidence/${analysisId}`);
  return response.data;
};

// Тренды
export const getTrendingTags = async (limit: number = 20): Promise<TrendingTag[]> => {
  const response = await api.get(`/trends/tags?limit=${limit}`);
  return response.data;
};

// Дашборд
export const getDashboardStats = async () => {
  const response = await api.get('/dashboard/stats');
  return response.data;
};