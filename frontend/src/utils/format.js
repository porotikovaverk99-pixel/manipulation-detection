import { SCORERS } from '../services/api';

export function summarize(items) {
  const total = items.length;
  const high = items.filter((item) => item.risk_level === 'high').length;
  const medium = items.filter((item) => item.risk_level === 'medium').length;
  const low = items.filter((item) => item.risk_level === 'low').length;
  const mean = total ? items.reduce((sum, item) => sum + (item.risk_score || 0), 0) / total : 0;
  return { total, high, medium, low, mean };
}

export function scoreToLevel(score) {
  if ((score || 0) >= 0.7) return 'high';
  if ((score || 0) >= 0.4) return 'medium';
  return 'low';
}

export function riskClass(level) {
  return level === 'medium' ? 'med' : level;
}

export function barHeight(value) {
  return `${4 + (value || 0) * 16}px`;
}

export function formatScore(value) {
  return value === null || value === undefined ? '-' : value.toFixed(2);
}

export function formatMetric(value) {
  return value === undefined ? '-' : value.toFixed(3);
}

export function formatPercent(value) {
  return `${Math.round(value * 100)}%`;
}

export function formatDate(value) {
  return value ? value.replace('T', ' ').replace('Z', '') : '-';
}

export function formatOffset(seconds) {
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
}

export function scoreLabel(key) {
  return SCORERS.find((item) => item.key === key)?.label || key;
}

export function bestBy(rows, key) {
  return rows.reduce((best, row) => {
    if (!best || (row.metrics[key] || 0) > (best.metrics[key] || 0)) return row;
    return best;
  }, null);
}

export function maxMetric(rows, key) {
  return Math.max(...rows.map((row) => row.metrics[key] || 0));
}
