import styles from './ScoreBar.module.css';
import { formatScore, riskClass } from '../../../utils/format';

export function ScoreBar({ value, level, compact = false }) {
  const width = Math.max(0, Math.min(1, value || 0)) * 100;

  return (
    <div className={`${styles.root} score-cell ${compact ? 'compact' : ''}`}>
      <div className={`score-track ${riskClass(level)}`}>
        <span style={{ width: `${width}%` }} />
      </div>
      {!compact && <span className="mono">{formatScore(value)}</span>}
    </div>
  );
}
